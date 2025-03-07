/*
Copyright 2019 The Cloud-Barista Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package mcir is to manage multi-cloud infra resource
package mcir

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	//uuid "github.com/google/uuid"
	"github.com/cloud-barista/cb-spider/interface/api"
	"github.com/cloud-barista/cb-tumblebug/src/core/common"
	"github.com/go-resty/resty/v2"

	// CB-Store
	cbstore_utils "github.com/cloud-barista/cb-store/utils"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"reflect"

	validator "github.com/go-playground/validator/v10"
)

// CB-Store
//var cblog *logrus.Logger
//var store icbs.Store

//var SPIDER_REST_URL string

// use a single instance of Validate, it caches struct info
var validate *validator.Validate

func init() {
	//cblog = config.Cblogger
	//store = cbstore.GetStore()
	//SPIDER_REST_URL = os.Getenv("SPIDER_REST_URL")

	validate = validator.New()

	// register function to get tag name from json tags.
	validate.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})

	// register validation for 'Tb*Req'
	// NOTE: only have to register a non-pointer type for 'Tb*Req', validator
	// internally dereferences during it's type checks.
	validate.RegisterStructValidation(TbImageReqStructLevelValidation, TbImageReq{})
	validate.RegisterStructValidation(TbSecurityGroupReqStructLevelValidation, TbSecurityGroupReq{})
	validate.RegisterStructValidation(TbSpecReqStructLevelValidation, TbSpecReq{})
	validate.RegisterStructValidation(TbSshKeyReqStructLevelValidation, TbSshKeyReq{})
	validate.RegisterStructValidation(TbVNetReqStructLevelValidation, TbVNetReq{})
}

// DelAllResources deletes all TB MCIR object of given resourceType
func DelAllResources(nsId string, resourceType string, forceFlag string) error {

	err := common.CheckString(nsId)
	if err != nil {
		common.CBLog.Error(err)
		return err
	}

	resourceIdList, err := ListResourceId(nsId, resourceType)
	if err != nil {
		return err
	}

	if len(resourceIdList) == 0 {
		return nil
	}

	for _, v := range resourceIdList {
		err := DelResource(nsId, resourceType, v, forceFlag)
		if err != nil {
			return err
		}
	}
	return nil
}

// DelResource deletes the TB MCIR object
func DelResource(nsId string, resourceType string, resourceId string, forceFlag string) error {

	fmt.Printf("DelResource() called; %s %s %s \n", nsId, resourceType, resourceId) // for debug

	err := common.CheckString(nsId)
	if err != nil {
		common.CBLog.Error(err)
		return err
	}

	err = common.CheckString(resourceId)
	if err != nil {
		common.CBLog.Error(err)
		return err
	}
	check, err := CheckResource(nsId, resourceType, resourceId)

	if err != nil {
		common.CBLog.Error(err)
		return err
	}

	if !check {
		errString := "The " + resourceType + " " + resourceId + " does not exist."
		//mapA := map[string]string{"message": errString}
		//mapB, _ := json.Marshal(mapA)
		err := fmt.Errorf(errString)
		//return http.StatusNotFound, mapB, err
		return err
	}

	key := common.GenResourceKey(nsId, resourceType, resourceId)
	fmt.Println("key: " + key)

	keyValue, _ := common.CBStore.Get(key)
	// In CheckResource() above, calling 'CBStore.Get()' and checking err parts exist.
	// So, in here, we don't need to check whether keyValue == nil or err != nil.

	//cspType := common.GetResourcesCspType(nsId, resourceType, resourceId)

	var childResources interface{}

	if os.Getenv("SPIDER_CALL_METHOD") == "REST" {

		var url string

		// Create Req body
		type JsonTemplate struct {
			ConnectionName string
		}
		tempReq := JsonTemplate{}

		switch resourceType {
		case common.StrImage:
			// delete image info
			err := common.CBStore.Delete(key)
			if err != nil {
				common.CBLog.Error(err)
				//return http.StatusInternalServerError, nil, err
				return err
			}

			// "DELETE FROM `image` WHERE `id` = '" + resourceId + "';"
			_, err = common.ORM.Delete(&TbImageInfo{Namespace: nsId, Id: resourceId})
			if err != nil {
				fmt.Println(err.Error())
			} else {
				fmt.Println("Data deleted successfully..")
			}

			//return http.StatusOK, nil, nil
			return nil
		case common.StrSpec:
			// delete spec info

			//get related recommend spec
			//keyValue, err := common.CBStore.Get(key)
			content := TbSpecInfo{}
			err := json.Unmarshal([]byte(keyValue.Value), &content)
			if err != nil {
				common.CBLog.Error(err)
				return err
			}

			err = common.CBStore.Delete(key)
			if err != nil {
				common.CBLog.Error(err)
				return err
			}

			// "DELETE FROM `spec` WHERE `id` = '" + resourceId + "';"
			_, err = common.ORM.Delete(&TbSpecInfo{Namespace: nsId, Id: resourceId})
			if err != nil {
				fmt.Println(err.Error())
			} else {
				fmt.Println("Data deleted successfully..")
			}

			//return http.StatusOK, nil, nil
			return nil
		case common.StrSSHKey:
			temp := TbSshKeyInfo{}
			err = json.Unmarshal([]byte(keyValue.Value), &temp)
			if err != nil {
				common.CBLog.Error(err)
				return err
			}
			tempReq.ConnectionName = temp.ConnectionName
			url = common.SpiderRestUrl + "/keypair/" + temp.Name
		case common.StrVNet:
			temp := TbVNetInfo{}
			err = json.Unmarshal([]byte(keyValue.Value), &temp)
			if err != nil {
				common.CBLog.Error(err)
				return err
			}
			tempReq.ConnectionName = temp.ConnectionName
			url = common.SpiderRestUrl + "/vpc/" + temp.Name
			childResources = temp.SubnetInfoList
		case common.StrSecurityGroup:
			temp := TbSecurityGroupInfo{}
			err = json.Unmarshal([]byte(keyValue.Value), &temp)
			if err != nil {
				common.CBLog.Error(err)
				return err
			}
			tempReq.ConnectionName = temp.ConnectionName
			url = common.SpiderRestUrl + "/securitygroup/" + temp.Name
		/*
			case "subnet":
				temp := subnetInfo{}
				json.Unmarshal([]byte(keyValue.Value), &content)
				return content.CspSubnetId
			case "publicIp":
				temp := publicIpInfo{}
				json.Unmarshal([]byte(keyValue.Value), &temp)
				tempReq.ConnectionName = temp.ConnectionName
				url = common.SPIDER_REST_URL + "/publicip/" + temp.CspPublicIpName
			case "vNic":
				temp := vNicInfo{}
				json.Unmarshal([]byte(keyValue.Value), &temp)
				tempReq.ConnectionName = temp.ConnectionName
				url = common.SPIDER_REST_URL + "/vnic/" + temp.CspVNicName
		*/
		default:
			err := fmt.Errorf("invalid resourceType")
			//return http.StatusBadRequest, nil, err
			return err
		}

		fmt.Println("url: " + url)

		client := resty.New().SetCloseConnection(true)

		resp, err := client.R().
			SetHeader("Content-Type", "application/json").
			SetBody(tempReq).
			//SetResult(&SpiderSpecInfo{}). // or SetResult(AuthSuccess{}).
			//SetError(&AuthError{}).       // or SetError(AuthError{}).
			Delete(url)

		if err != nil {
			common.CBLog.Error(err)
			err := fmt.Errorf("an error occurred while requesting to CB-Spider")
			return err
		}

		fmt.Println("HTTP Status code: " + strconv.Itoa(resp.StatusCode()))
		switch {
		case forceFlag == "true":
			url += "?force=true"
			fmt.Println("forceFlag == true; url: " + url)

			_, err := client.R().
				SetHeader("Content-Type", "application/json").
				SetBody(tempReq).
				//SetResult(&SpiderSpecInfo{}). // or SetResult(AuthSuccess{}).
				//SetError(&AuthError{}).       // or SetError(AuthError{}).
				Delete(url)

			if err != nil {
				common.CBLog.Error(err)
				err := fmt.Errorf("an error occurred while requesting to CB-Spider")
				return err
			}

			// err = common.CBStore.Delete(key)
			// if err != nil {
			// 	common.CBLog.Error(err)
			// 	return err
			// }
			// return nil
		case resp.StatusCode() >= 400 || resp.StatusCode() < 200:
			err := fmt.Errorf(string(resp.Body()))
			common.CBLog.Error(err)
			return err
		default:
			// err := common.CBStore.Delete(key)
			// if err != nil {
			// 	common.CBLog.Error(err)
			// 	return err
			// }
			// return nil
		}

	} else {

		// Set CCM gRPC API
		ccm := api.NewCloudResourceHandler()
		err := ccm.SetConfigPath(os.Getenv("CBTUMBLEBUG_ROOT") + "/conf/grpc_conf.yaml")
		if err != nil {
			common.CBLog.Error("ccm failed to set config : ", err)
			return err
		}
		err = ccm.Open()
		if err != nil {
			common.CBLog.Error("ccm api open failed : ", err)
			return err
		}
		defer ccm.Close()

		switch resourceType {
		case common.StrImage:
			// delete image info
			err := common.CBStore.Delete(key)
			if err != nil {
				common.CBLog.Error(err)
				//return http.StatusInternalServerError, nil, err
				return err
			}

			// "DELETE FROM `image` WHERE `id` = '" + resourceId + "';"
			_, err = common.ORM.Delete(&TbImageInfo{Namespace: nsId, Id: resourceId})
			if err != nil {
				fmt.Println(err.Error())
			} else {
				fmt.Println("Data deleted successfully..")
			}

			//return http.StatusOK, nil, nil
			return nil
		case common.StrSpec:
			// delete spec info

			//get related recommend spec
			content := TbSpecInfo{}
			err := json.Unmarshal([]byte(keyValue.Value), &content)
			if err != nil {
				common.CBLog.Error(err)
				return err
			}

			err = common.CBStore.Delete(key)
			if err != nil {
				common.CBLog.Error(err)
				return err
			}

			// "DELETE FROM `spec` WHERE `id` = '" + resourceId + "';"
			_, err = common.ORM.Delete(&TbSpecInfo{Namespace: nsId, Id: resourceId})
			if err != nil {
				fmt.Println(err.Error())
			} else {
				fmt.Println("Data deleted successfully..")
			}

			return nil

		case common.StrSSHKey:
			temp := TbSshKeyInfo{}
			err := json.Unmarshal([]byte(keyValue.Value), &temp)
			if err != nil {
				common.CBLog.Error(err)
				return err
			}

			_, err = ccm.DeleteKeyByParam(temp.ConnectionName, temp.Name, forceFlag)
			if err != nil {
				common.CBLog.Error(err)
				return err
			}

		case common.StrVNet:
			temp := TbVNetInfo{}
			err := json.Unmarshal([]byte(keyValue.Value), &temp)
			if err != nil {
				common.CBLog.Error(err)
				return err
			}

			_, err = ccm.DeleteVPCByParam(temp.ConnectionName, temp.Name, forceFlag)
			if err != nil {
				common.CBLog.Error(err)
				return err
			}

			childResources = temp.SubnetInfoList
		case common.StrSecurityGroup:
			temp := TbSecurityGroupInfo{}
			err := json.Unmarshal([]byte(keyValue.Value), &temp)
			if err != nil {
				common.CBLog.Error(err)
				return err
			}

			_, err = ccm.DeleteSecurityByParam(temp.ConnectionName, temp.Name, forceFlag)
			if err != nil {
				common.CBLog.Error(err)
				return err
			}

		default:
			err := fmt.Errorf("invalid resourceType")
			return err
		}

		// err = common.CBStore.Delete(key)
		// if err != nil {
		// 	common.CBLog.Error(err)
		// 	return err
		// }
		// return nil

	}

	if resourceType == common.StrVNet {
		// var subnetKeys []string
		fmt.Printf("childResources: %s", childResources) // for debug
		subnets := childResources.([]TbSubnetInfo)
		for _, v := range subnets {
			subnetKey := common.GenChildResourceKey(nsId, common.StrSubnet, resourceId, v.Id)
			// subnetKeys = append(subnetKeys, subnetKey)
			fmt.Printf("subnetKey: %s", subnetKey) // for debug
			err = common.CBStore.Delete(subnetKey)
			if err != nil {
				common.CBLog.Error(err)
				// return err
			}
		}
	}

	err = common.CBStore.Delete(key)
	if err != nil {
		common.CBLog.Error(err)
		return err
	}
	return nil
}

// DelChildResource deletes the TB MCIR object
func DelChildResource(nsId string, resourceType string, parentResourceId string, resourceId string, forceFlag string) error {

	fmt.Printf("DelChildResource() called; %s %s %s %s \n", nsId, resourceType, parentResourceId, resourceId) // for debug

	var parentResourceType string
	switch resourceType {
	case common.StrSubnet:
		parentResourceType = common.StrVNet
	default:
		err := fmt.Errorf("Not valid child resource type.")
		return err
	}

	err := common.CheckString(nsId)
	if err != nil {
		common.CBLog.Error(err)
		return err
	}

	err = common.CheckString(parentResourceId)
	if err != nil {
		common.CBLog.Error(err)
		return err
	}

	err = common.CheckString(resourceId)
	if err != nil {
		common.CBLog.Error(err)
		return err
	}

	check, err := CheckResource(nsId, parentResourceType, parentResourceId)

	if !check {
		errString := "The " + parentResourceType + " " + parentResourceId + " does not exist."
		//mapA := map[string]string{"message": errString}
		//mapB, _ := json.Marshal(mapA)
		err := fmt.Errorf(errString)
		//return http.StatusNotFound, mapB, err
		return err
	}

	if err != nil {
		common.CBLog.Error(err)
		return err
	}

	check, err = CheckChildResource(nsId, resourceType, parentResourceId, resourceId)

	if !check {
		errString := "The " + resourceType + " " + resourceId + " does not exist."
		//mapA := map[string]string{"message": errString}
		//mapB, _ := json.Marshal(mapA)
		err := fmt.Errorf(errString)
		//return http.StatusNotFound, mapB, err
		return err
	}

	if err != nil {
		common.CBLog.Error(err)
		return err
	}

	parentResourceKey := common.GenResourceKey(nsId, parentResourceType, parentResourceId)
	fmt.Println("parentResourceKey: " + parentResourceKey)

	childResourceKey := common.GenChildResourceKey(nsId, resourceType, parentResourceId, resourceId)
	fmt.Println("childResourceKey: " + childResourceKey)

	parentKeyValue, _ := common.CBStore.Get(parentResourceKey)
	/*
		if keyValue == nil {
			mapA := map[string]string{"message": "Failed to find the resource with given ID."}
			mapB, _ := json.Marshal(mapA)
			err := fmt.Errorf("Failed to find the resource with given ID.")
			return http.StatusNotFound, mapB, err
		}
	*/
	//fmt.Println("keyValue: " + keyValue.Key + " / " + keyValue.Value)

	//cspType := common.GetResourcesCspType(nsId, resourceType, resourceId)

	if os.Getenv("SPIDER_CALL_METHOD") == "REST" {

		var url string

		// Create Req body
		type JsonTemplate struct {
			ConnectionName string
		}
		tempReq := JsonTemplate{}

		switch resourceType {
		case common.StrSubnet:
			temp := TbVNetInfo{}
			err = json.Unmarshal([]byte(parentKeyValue.Value), &temp)
			if err != nil {
				common.CBLog.Error(err)
				return err
			}
			tempReq.ConnectionName = temp.ConnectionName
			// url = common.SpiderRestUrl + "/vpc/" + temp.Name
			url = fmt.Sprintf("%s/vpc/%s/subnet/%s", common.SpiderRestUrl, temp.Name, resourceId)
		default:
			err := fmt.Errorf("invalid resourceType")
			//return http.StatusBadRequest, nil, err
			return err
		}

		fmt.Println("url: " + url)

		client := resty.New().SetCloseConnection(true)

		resp, err := client.R().
			SetHeader("Content-Type", "application/json").
			SetBody(tempReq).
			//SetResult(&SpiderSpecInfo{}). // or SetResult(AuthSuccess{}).
			//SetError(&AuthError{}).       // or SetError(AuthError{}).
			Delete(url)

		if err != nil {
			common.CBLog.Error(err)
			err := fmt.Errorf("an error occurred while requesting to CB-Spider")
			return err
		}

		fmt.Println("HTTP Status code: " + strconv.Itoa(resp.StatusCode()))
		switch {
		case forceFlag == "true":
			url += "?force=true"
			fmt.Println("forceFlag == true; url: " + url)

			_, err := client.R().
				SetHeader("Content-Type", "application/json").
				SetBody(tempReq).
				//SetResult(&SpiderSpecInfo{}). // or SetResult(AuthSuccess{}).
				//SetError(&AuthError{}).       // or SetError(AuthError{}).
				Delete(url)

			if err != nil {
				common.CBLog.Error(err)
				err := fmt.Errorf("an error occurred while requesting to CB-Spider")
				return err
			}

		case resp.StatusCode() >= 400 || resp.StatusCode() < 200:
			err := fmt.Errorf(string(resp.Body()))
			common.CBLog.Error(err)
			return err
		default:

		}

	} else {

		// Set CCM gRPC API
		ccm := api.NewCloudResourceHandler()
		err := ccm.SetConfigPath(os.Getenv("CBTUMBLEBUG_ROOT") + "/conf/grpc_conf.yaml")
		if err != nil {
			common.CBLog.Error("ccm failed to set config : ", err)
			return err
		}
		err = ccm.Open()
		if err != nil {
			common.CBLog.Error("ccm api open failed : ", err)
			return err
		}
		defer ccm.Close()

		switch resourceType {
		case common.StrSubnet:
			temp := TbVNetInfo{}
			err := json.Unmarshal([]byte(parentKeyValue.Value), &temp)
			if err != nil {
				common.CBLog.Error(err)
				return err
			}

			_, err = ccm.RemoveSubnetByParam(temp.ConnectionName, temp.Name, resourceId, forceFlag)
			if err != nil {
				common.CBLog.Error(err)
				return err
			}
		default:
			err := fmt.Errorf("invalid resourceType")
			return err
		}

	}

	err = common.CBStore.Delete(childResourceKey)
	if err != nil {
		common.CBLog.Error(err)
		return err
	}

	// Delete the child element in parent resources' array
	switch resourceType {
	case common.StrSubnet:
		oldVNet := TbVNetInfo{}
		err = json.Unmarshal([]byte(parentKeyValue.Value), &oldVNet)
		if err != nil {
			common.CBLog.Error(err)
			return err
		}

		newVNet := TbVNetInfo{}
		newVNet = oldVNet

		var subnetIndex int
		subnetIndex = -1
		for i, v := range newVNet.SubnetInfoList {
			if v.Name == resourceId {
				subnetIndex = i
				break
			}
		}

		if subnetIndex != -1 {
			DelEleInSlice(&newVNet.SubnetInfoList, subnetIndex)
		} else {
			err := fmt.Errorf("Failed to find and delete subnet %s in vNet %s.", resourceId, parentResourceId)
			common.CBLog.Error(err)
		}

		Val, _ := json.Marshal(newVNet)
		err = common.CBStore.Put(parentResourceKey, string(Val))
		if err != nil {
			common.CBLog.Error(err)
			return err
		}
		// default:
	}

	return nil

}

// DelEleInSlice delete an element from slice by index
//  - arr: the reference of slice
//  - index: the index of element will be deleted
func DelEleInSlice(arr interface{}, index int) {
	vField := reflect.ValueOf(arr)
	value := vField.Elem()
	if value.Kind() == reflect.Slice || value.Kind() == reflect.Array {
		result := reflect.AppendSlice(value.Slice(0, index), value.Slice(index+1, value.Len()))
		value.Set(result)
	}
}

// SpiderNameIdSystemId is struct for mapping NameID and System ID from CB-Spider response
type SpiderNameIdSystemId struct {
	NameId   string
	SystemId string
}

// SpiderAllListWrapper is struct for wrapping SpiderAllList struct from CB-Spider response.
type SpiderAllListWrapper struct {
	AllList SpiderAllList
}

// SpiderAllList is struct for OnlyCSPList, OnlySpiderList MappedList from CB-Spider response.
type SpiderAllList struct {
	MappedList     []SpiderNameIdSystemId
	OnlySpiderList []SpiderNameIdSystemId
	OnlyCSPList    []SpiderNameIdSystemId
}

// TbInspectResourcesResponse is struct for response of InspectResources request
type TbInspectResourcesResponse struct {
	// ResourcesOnCsp       interface{} `json:"resourcesOnCsp"`
	// ResourcesOnSpider    interface{} `json:"resourcesOnSpider"`
	// ResourcesOnTumblebug interface{} `json:"resourcesOnTumblebug"`
	ResourcesOnCsp       []resourceOnCspOrSpider `json:"resourcesOnCsp"`
	ResourcesOnSpider    []resourceOnCspOrSpider `json:"resourcesOnSpider"`
	ResourcesOnTumblebug []resourceOnTumblebug   `json:"resourcesOnTumblebug"`
}

type resourceOnCspOrSpider struct {
	Id          string `json:"id"`
	CspNativeId string `json:"cspNativeId"`
}

type resourceOnTumblebug struct {
	Id          string `json:"id"`
	CspNativeId string `json:"cspNativeId"`
	NsId        string `json:"nsId"`
	//McisId      string `json:"mcisId"`
	Type      string `json:"type"`
	ObjectKey string `json:"objectKey"`
}

// InspectResources returns the state list of TB MCIR objects of given connConfig and resourceType
func InspectResources(connConfig string, resourceType string) (interface{}, error) {

	nsList, err := common.ListNsId()
	if err != nil {
		common.CBLog.Error(err)
		err = fmt.Errorf("an error occurred while getting namespaces' list: " + err.Error())
		return nil, err
	}
	// var TbResourceList []string
	var TbResourceList []resourceOnTumblebug
	for _, ns := range nsList {
		/*
			resourceListInNs := ListResourceId(ns, resourceType)
			for i, _ := range resourceListInNs {
				resourceListInNs[i] = ns + "/" + resourceListInNs[i]
			}
			TbResourceList = append(TbResourceList, resourceListInNs...)
		*/

		resourceListInNs, err := ListResource(ns, resourceType)
		if err != nil {
			common.CBLog.Error(err)
			err := fmt.Errorf("an error occurred while getting resource list")
			return nil, err
		}
		if resourceListInNs == nil {
			continue
		}

		switch resourceType {
		case common.StrVNet:
			resourcesInNs := resourceListInNs.([]TbVNetInfo) // type assertion
			for _, resource := range resourcesInNs {
				if resource.ConnectionName == connConfig { // filtering
					temp := resourceOnTumblebug{}
					temp.Id = resource.Id
					temp.CspNativeId = resource.CspVNetId
					temp.NsId = ns
					//temp.McisId = ""
					temp.Type = resourceType
					temp.ObjectKey = common.GenResourceKey(ns, resourceType, resource.Id)

					TbResourceList = append(TbResourceList, temp)
				}
			}
		case common.StrSecurityGroup:
			resourcesInNs := resourceListInNs.([]TbSecurityGroupInfo) // type assertion
			for _, resource := range resourcesInNs {
				if resource.ConnectionName == connConfig { // filtering
					temp := resourceOnTumblebug{}
					temp.Id = resource.Id
					temp.CspNativeId = resource.CspSecurityGroupId
					temp.NsId = ns
					//temp.McisId = ""
					temp.Type = resourceType
					temp.ObjectKey = common.GenResourceKey(ns, resourceType, resource.Id)

					TbResourceList = append(TbResourceList, temp)
				}
			}
		case common.StrSSHKey:
			resourcesInNs := resourceListInNs.([]TbSshKeyInfo) // type assertion
			for _, resource := range resourcesInNs {
				if resource.ConnectionName == connConfig { // filtering
					temp := resourceOnTumblebug{}
					temp.Id = resource.Id
					temp.CspNativeId = resource.CspSshKeyName
					temp.NsId = ns
					//temp.McisId = ""
					temp.Type = resourceType
					temp.ObjectKey = common.GenResourceKey(ns, resourceType, resource.Id)

					TbResourceList = append(TbResourceList, temp)
				}
			}
		}
	}

	client := resty.New().SetCloseConnection(true)
	client.SetAllowGetMethodPayload(true)

	// Create Req body
	type JsonTemplate struct {
		ConnectionName string
	}
	tempReq := JsonTemplate{}
	tempReq.ConnectionName = connConfig

	var spiderRequestURL string
	switch resourceType {
	case common.StrVNet:
		spiderRequestURL = common.SpiderRestUrl + "/allvpc"
	case common.StrSecurityGroup:
		spiderRequestURL = common.SpiderRestUrl + "/allsecuritygroup"
	case common.StrSSHKey:
		spiderRequestURL = common.SpiderRestUrl + "/allkeypair"
	}

	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(tempReq).
		SetResult(&SpiderAllListWrapper{}). // or SetResult(AuthSuccess{}).
		//SetError(&AuthError{}).       // or SetError(AuthError{}).
		Get(spiderRequestURL)

	if err != nil {
		common.CBLog.Error(err)
		err := fmt.Errorf("an error occurred while requesting to CB-Spider")
		return nil, err
	}

	fmt.Println("HTTP Status code: " + strconv.Itoa(resp.StatusCode()))
	switch {
	case resp.StatusCode() >= 400 || resp.StatusCode() < 200:
		err := fmt.Errorf(string(resp.Body()))
		common.CBLog.Error(err)
		return nil, err
	default:
	}

	temp, _ := resp.Result().(*SpiderAllListWrapper) // type assertion

	result := TbInspectResourcesResponse{}

	/*
		// Implementation style 1
		if len(TbResourceList) > 0 {
			result.ResourcesOnTumblebug = TbResourceList
		} else {
			result.ResourcesOnTumblebug = []resourceOnTumblebug{}
		}
	*/
	// Implementation style 2
	result.ResourcesOnTumblebug = []resourceOnTumblebug{}
	result.ResourcesOnTumblebug = append(result.ResourcesOnTumblebug, TbResourceList...)

	// result.ResourcesOnCsp = append((*temp).AllList.MappedList, (*temp).AllList.OnlyCSPList...)
	// result.ResourcesOnSpider = append((*temp).AllList.MappedList, (*temp).AllList.OnlySpiderList...)
	result.ResourcesOnCsp = []resourceOnCspOrSpider{}
	result.ResourcesOnSpider = []resourceOnCspOrSpider{}

	for _, v := range (*temp).AllList.MappedList {
		tmpObj := resourceOnCspOrSpider{}
		tmpObj.Id = v.NameId
		tmpObj.CspNativeId = v.SystemId

		result.ResourcesOnCsp = append(result.ResourcesOnCsp, tmpObj)
		result.ResourcesOnSpider = append(result.ResourcesOnSpider, tmpObj)
	}

	for _, v := range (*temp).AllList.OnlySpiderList {
		tmpObj := resourceOnCspOrSpider{}
		tmpObj.Id = v.NameId
		tmpObj.CspNativeId = v.SystemId

		result.ResourcesOnSpider = append(result.ResourcesOnSpider, tmpObj)
	}

	for _, v := range (*temp).AllList.OnlyCSPList {
		tmpObj := resourceOnCspOrSpider{}
		tmpObj.Id = v.NameId
		tmpObj.CspNativeId = v.SystemId

		result.ResourcesOnCsp = append(result.ResourcesOnCsp, tmpObj)
	}

	return result, nil
}

// ListResourceId returns the list of TB MCIR object IDs of given resourceType
func ListResourceId(nsId string, resourceType string) ([]string, error) {

	err := common.CheckString(nsId)
	if err != nil {
		common.CBLog.Error(err)
		return nil, err
	}

	if resourceType == common.StrImage ||
		resourceType == common.StrSSHKey ||
		resourceType == common.StrSpec ||
		resourceType == common.StrVNet ||
		//resourceType == "subnet" ||
		//resourceType == "publicIp" ||
		//resourceType == "vNic" ||
		resourceType == common.StrSecurityGroup {
		// continue
	} else {
		err = fmt.Errorf("invalid resource type")
		common.CBLog.Error(err)
		return nil, err
	}

	fmt.Println("[ListResourceId] ns: " + nsId + ", type: " + resourceType)
	key := "/ns/" + nsId + "/resources/"
	fmt.Println(key)

	keyValue, err := common.CBStore.GetList(key, true)

	if err != nil {
		common.CBLog.Error(err)
		return nil, err
	}

	/* if keyValue == nil, then for-loop below will not be executed, and the empty array will be returned in `resourceList` placeholder.
	if keyValue == nil {
		err = fmt.Errorf("ListResourceId(); %s is empty.", key)
		common.CBLog.Error(err)
		return nil, err
	}
	*/

	var resourceList []string
	for _, v := range keyValue {
		trimmedString := strings.TrimPrefix(v.Key, (key + resourceType + "/"))
		// prevent malformed key (if key for resource id includes '/', the key does not represent resource ID)
		if !strings.Contains(trimmedString, "/") {
			resourceList = append(resourceList, trimmedString)
		}
	}
	// for _, v := range resourceList {
	// 	fmt.Println("<" + v + "> \n")
	// }
	// fmt.Println("===============================================")
	return resourceList, nil

}

// ListResource returns the list of TB MCIR objects of given resourceType
func ListResource(nsId string, resourceType string) (interface{}, error) {

	err := common.CheckString(nsId)
	if err != nil {
		common.CBLog.Error(err)
		return nil, err
	}

	if resourceType == common.StrImage ||
		resourceType == common.StrSSHKey ||
		resourceType == common.StrSpec ||
		resourceType == common.StrVNet ||
		//resourceType == "subnet" ||
		//resourceType == "publicIp" ||
		//resourceType == "vNic" ||
		resourceType == common.StrSecurityGroup {
		// continue
	} else {
		errString := "Cannot list " + resourceType + "s."
		err := fmt.Errorf(errString)
		return nil, err
	}

	fmt.Println("[Get " + resourceType + " list")
	key := "/ns/" + nsId + "/resources/" + resourceType
	fmt.Println(key)

	keyValue, err := common.CBStore.GetList(key, true)
	keyValue = cbstore_utils.GetChildList(keyValue, key)

	if err != nil {
		common.CBLog.Error(err)
		/*
			fmt.Println("func ListResource; common.CBStore.GetList gave error")
			var resourceList []string
			for _, v := range keyValue {
				resourceList = append(resourceList, strings.TrimPrefix(v.Key, "/ns/"+nsId+"/resources/"+resourceType+"/"))
			}
			for _, v := range resourceList {
				fmt.Println("<" + v + "> \n")
			}
			fmt.Println("===============================================")
		*/
		return nil, err
	}
	if keyValue != nil {
		switch resourceType {
		case common.StrImage:
			res := []TbImageInfo{}
			for _, v := range keyValue {
				tempObj := TbImageInfo{}
				err = json.Unmarshal([]byte(v.Value), &tempObj)
				if err != nil {
					common.CBLog.Error(err)
					return nil, err
				}
				res = append(res, tempObj)
			}
			return res, nil
		case common.StrSecurityGroup:
			res := []TbSecurityGroupInfo{}
			for _, v := range keyValue {
				tempObj := TbSecurityGroupInfo{}
				err = json.Unmarshal([]byte(v.Value), &tempObj)
				if err != nil {
					common.CBLog.Error(err)
					return nil, err
				}
				res = append(res, tempObj)
			}
			return res, nil
		case common.StrSpec:
			res := []TbSpecInfo{}
			for _, v := range keyValue {
				tempObj := TbSpecInfo{}
				err = json.Unmarshal([]byte(v.Value), &tempObj)
				if err != nil {
					common.CBLog.Error(err)
					return nil, err
				}
				res = append(res, tempObj)
			}
			return res, nil
		case common.StrSSHKey:
			res := []TbSshKeyInfo{}
			for _, v := range keyValue {
				tempObj := TbSshKeyInfo{}
				err = json.Unmarshal([]byte(v.Value), &tempObj)
				if err != nil {
					common.CBLog.Error(err)
					return nil, err
				}
				res = append(res, tempObj)
			}
			return res, nil
		case common.StrVNet:
			res := []TbVNetInfo{}
			for _, v := range keyValue {
				tempObj := TbVNetInfo{}
				err = json.Unmarshal([]byte(v.Value), &tempObj)
				if err != nil {
					common.CBLog.Error(err)
					return nil, err
				}
				res = append(res, tempObj)
			}
			return res, nil
		}

		//return true, nil
	}

	return nil, nil // When err == nil && keyValue == nil
}

// GetAssociatedObjectCount returns the number of MCIR's associated Tumblebug objects
func GetAssociatedObjectCount(nsId string, resourceType string, resourceId string) (int, error) {

	err := common.CheckString(nsId)
	if err != nil {
		common.CBLog.Error(err)
		return -1, err
	}

	err = common.CheckString(resourceId)
	if err != nil {
		common.CBLog.Error(err)
		return -1, err
	}
	check, err := CheckResource(nsId, resourceType, resourceId)

	if !check {
		errString := "The " + resourceType + " " + resourceId + " does not exist."
		//mapA := map[string]string{"message": errString}
		//mapB, _ := json.Marshal(mapA)
		err := fmt.Errorf(errString)
		return -1, err
	}

	if err != nil {
		common.CBLog.Error(err)
		return -1, err
	}
	fmt.Println("[Get count] " + resourceType + ", " + resourceId)

	key := common.GenResourceKey(nsId, resourceType, resourceId)
	//fmt.Println(key)

	keyValue, err := common.CBStore.Get(key)
	if err != nil {
		common.CBLog.Error(err)
		return -1, err
	}
	if keyValue != nil {
		inUseCount := int(gjson.Get(keyValue.Value, "associatedObjectList.#").Int())
		return inUseCount, nil
	}
	errString := "Cannot get " + resourceType + " " + resourceId + "."
	err = fmt.Errorf(errString)
	return -1, err
}

// GetAssociatedObjectList returns the list of MCIR's associated Tumblebug objects
func GetAssociatedObjectList(nsId string, resourceType string, resourceId string) ([]string, error) {

	var result []string

	err := common.CheckString(nsId)
	if err != nil {
		common.CBLog.Error(err)
		return nil, err
	}

	err = common.CheckString(resourceId)
	if err != nil {
		common.CBLog.Error(err)
		return nil, err
	}
	check, err := CheckResource(nsId, resourceType, resourceId)

	if !check {
		errString := "The " + resourceType + " " + resourceId + " does not exist."
		//mapA := map[string]string{"message": errString}
		//mapB, _ := json.Marshal(mapA)
		err := fmt.Errorf(errString)
		return nil, err
	}

	if err != nil {
		common.CBLog.Error(err)
		return nil, err
	}
	fmt.Println("[Get count] " + resourceType + ", " + resourceId)

	key := common.GenResourceKey(nsId, resourceType, resourceId)
	//fmt.Println(key)

	keyValue, err := common.CBStore.Get(key)
	if err != nil {
		common.CBLog.Error(err)
		return nil, err
	}
	if keyValue != nil {
		/*
			objList := gjson.Get(keyValue.Value, "associatedObjectList")
			objList.ForEach(func(key, value gjson.Result) bool {
				result = append(result, value.String())
				return true
			})
		*/

		/*
			switch resourceType {
			case common.StrImage:
				res := TbImageInfo{}
				json.Unmarshal([]byte(keyValue.Value), &res)
				//result = res.
			case common.StrSecurityGroup:
				res := TbSecurityGroupInfo{}
				json.Unmarshal([]byte(keyValue.Value), &res)

			case common.StrSpec:
				res := TbSpecInfo{}
				json.Unmarshal([]byte(keyValue.Value), &res)

			case common.StrSSHKey:
				res := TbSshKeyInfo{}
				json.Unmarshal([]byte(keyValue.Value), &res)
				result = res.AssociatedObjectList
			case common.StrVNet:
				res := TbVNetInfo{}
				json.Unmarshal([]byte(keyValue.Value), &res)

			}
		*/

		type stringList struct {
			AssociatedObjectList []string `json:"associatedObjectList"`
		}
		res := stringList{}
		err = json.Unmarshal([]byte(keyValue.Value), &res)
		if err != nil {
			common.CBLog.Error(err)
			return nil, err
		}
		result = res.AssociatedObjectList

		return result, nil
	}
	errString := "Cannot get " + resourceType + " " + resourceId + "."
	err = fmt.Errorf(errString)
	return nil, err
}

// UpdateAssociatedObjectList adds or deletes the objectKey (currently, vmKey) to/from TB object's associatedObjectList
func UpdateAssociatedObjectList(nsId string, resourceType string, resourceId string, cmd string, objectKey string) ([]string, error) {

	err := common.CheckString(nsId)
	if err != nil {
		common.CBLog.Error(err)
		return nil, err
	}

	err = common.CheckString(resourceId)
	if err != nil {
		common.CBLog.Error(err)
		return nil, err
	}
	/*
		check, err := CheckResource(nsId, resourceType, resourceId)

		if !check {
			errString := "The " + resourceType + " " + resourceId + " does not exist."
			//mapA := map[string]string{"message": errString}
			//mapB, _ := json.Marshal(mapA)
			err := fmt.Errorf(errString)
			return -1, err
		}

		if err != nil {
			common.CBLog.Error(err)
			return -1, err
		}
	*/
	fmt.Println("[Set count] " + resourceType + ", " + resourceId)

	key := common.GenResourceKey(nsId, resourceType, resourceId)
	//fmt.Println(key)

	keyValue, err := common.CBStore.Get(key)
	if err != nil {
		common.CBLog.Error(err)
		return nil, err
	}

	if keyValue != nil {
		objList, _ := GetAssociatedObjectList(nsId, resourceType, resourceId)
		switch cmd {
		case common.StrAdd:
			for _, v := range objList {
				if v == objectKey {
					errString := objectKey + " is already associated with " + resourceType + " " + resourceId + "."
					err = fmt.Errorf(errString)
					return nil, err
				}
			}
			// fmt.Println("len(objList): " + strconv.Itoa(len(objList))) // for debug
			// fmt.Print("objList: ")                                     // for debug
			// fmt.Println(objList)                                       // for debug

			var anyJson map[string]interface{}
			json.Unmarshal([]byte(keyValue.Value), &anyJson)
			if anyJson["associatedObjectList"] == nil {
				arrayToBe := []string{objectKey}
				// fmt.Println("array_to_be: ", array_to_be) // for debug

				anyJson["associatedObjectList"] = arrayToBe
			} else { // anyJson["associatedObjectList"] != nil
				arrayAsIs := anyJson["associatedObjectList"].([]interface{})
				// fmt.Println("array_as_is: ", array_as_is) // for debug

				arrayToBe := append(arrayAsIs, objectKey)
				// fmt.Println("array_to_be: ", array_to_be) // for debug

				anyJson["associatedObjectList"] = arrayToBe
			}
			updatedJson, _ := json.Marshal(anyJson)
			// fmt.Println(string(updatedJson)) // for debug

			keyValue.Value = string(updatedJson)
		case common.StrDelete:
			var foundKey int
			var foundVal string
			for k, v := range objList {
				if v == objectKey {
					foundKey = k
					foundVal = v
					break
				}
			}
			if foundVal == "" {
				errString := "Cannot find the associated object " + objectKey + "."
				err = fmt.Errorf(errString)
				return nil, err
			} else {
				keyValue.Value, err = sjson.Delete(keyValue.Value, "associatedObjectList."+strconv.Itoa(foundKey))
				if err != nil {
					common.CBLog.Error(err)
					return nil, err
				}
			}
		}

		if err != nil {
			common.CBLog.Error(err)
			return nil, err
		}
		err = common.CBStore.Put(key, keyValue.Value)
		if err != nil {
			common.CBLog.Error(err)
			return nil, err
		}

		result, _ := GetAssociatedObjectList(nsId, resourceType, resourceId)
		return result, nil
	}
	errString := "Cannot get " + resourceType + " " + resourceId + "."
	err = fmt.Errorf(errString)
	return nil, err
}

// GetResource returns the requested TB MCIR object
func GetResource(nsId string, resourceType string, resourceId string) (interface{}, error) {

	err := common.CheckString(nsId)
	if err != nil {
		common.CBLog.Error(err)
		return nil, err
	}

	err = common.CheckString(resourceId)
	if err != nil {
		common.CBLog.Error(err)
		return nil, err
	}
	check, err := CheckResource(nsId, resourceType, resourceId)
	if err != nil {
		common.CBLog.Error(err)
		return nil, err
	}

	if !check {
		errString := "The " + resourceType + " " + resourceId + " does not exist."
		//mapA := map[string]string{"message": errString}
		//mapB, _ := json.Marshal(mapA)
		err := fmt.Errorf(errString)
		return nil, err
	}

	fmt.Println("[Get resource] " + resourceType + ", " + resourceId)

	key := common.GenResourceKey(nsId, resourceType, resourceId)
	//fmt.Println(key)

	keyValue, err := common.CBStore.Get(key)
	if err != nil {
		common.CBLog.Error(err)
		return nil, err
	}
	if keyValue != nil {
		switch resourceType {
		case common.StrImage:
			res := TbImageInfo{}
			err = json.Unmarshal([]byte(keyValue.Value), &res)
			if err != nil {
				common.CBLog.Error(err)
				return nil, err
			}
			return res, nil
		case common.StrSecurityGroup:
			res := TbSecurityGroupInfo{}
			err = json.Unmarshal([]byte(keyValue.Value), &res)
			if err != nil {
				common.CBLog.Error(err)
				return nil, err
			}
			return res, nil
		case common.StrSpec:
			res := TbSpecInfo{}
			err = json.Unmarshal([]byte(keyValue.Value), &res)
			if err != nil {
				common.CBLog.Error(err)
				return nil, err
			}
			return res, nil
		case common.StrSSHKey:
			res := TbSshKeyInfo{}
			err = json.Unmarshal([]byte(keyValue.Value), &res)
			if err != nil {
				common.CBLog.Error(err)
				return nil, err
			}
			return res, nil
		case common.StrVNet:
			res := TbVNetInfo{}
			err = json.Unmarshal([]byte(keyValue.Value), &res)
			if err != nil {
				common.CBLog.Error(err)
				return nil, err
			}
			return res, nil
		}

		//return true, nil
	}
	errString := "Cannot get " + resourceType + " " + resourceId + "."
	err = fmt.Errorf(errString)
	return nil, err
}

// CheckResource returns the existence of the TB MCIR resource in bool form.
func CheckResource(nsId string, resourceType string, resourceId string) (bool, error) {

	// Check parameters' emptiness
	if nsId == "" {
		err := fmt.Errorf("CheckResource failed; nsId given is null.")
		return false, err
	} else if resourceType == "" {
		err := fmt.Errorf("CheckResource failed; resourceType given is null.")
		return false, err
	} else if resourceId == "" {
		err := fmt.Errorf("CheckResource failed; resourceId given is null.")
		return false, err
	}

	// Check resourceType's validity
	if resourceType == common.StrImage ||
		resourceType == common.StrSSHKey ||
		resourceType == common.StrSpec ||
		resourceType == common.StrVNet ||
		resourceType == common.StrSecurityGroup {
		//resourceType == "subnet" ||
		//resourceType == "publicIp" ||
		//resourceType == "vNic" {
		// continue
	} else {
		err := fmt.Errorf("invalid resource type")
		return false, err
	}

	err := common.CheckString(nsId)
	if err != nil {
		common.CBLog.Error(err)
		return false, err
	}

	err = common.CheckString(resourceId)
	if err != nil {
		common.CBLog.Error(err)
		return false, err
	}

	fmt.Println("[Check resource] " + resourceType + ", " + resourceId)

	key := common.GenResourceKey(nsId, resourceType, resourceId)
	//fmt.Println(key)

	keyValue, err := common.CBStore.Get(key)
	if err != nil {
		common.CBLog.Error(err)
		return false, err
	}
	if keyValue != nil {
		return true, nil
	}
	return false, nil

}

// CheckChildResource returns the existence of the TB MCIR resource in bool form.
func CheckChildResource(nsId string, resourceType string, parentResourceId string, resourceId string) (bool, error) {

	// Check parameters' emptiness
	if nsId == "" {
		err := fmt.Errorf("CheckResource failed; nsId given is null.")
		return false, err
	} else if resourceType == "" {
		err := fmt.Errorf("CheckResource failed; resourceType given is null.")
		return false, err
	} else if parentResourceId == "" {
		err := fmt.Errorf("CheckResource failed; parentResourceId given is null.")
		return false, err
	} else if resourceId == "" {
		err := fmt.Errorf("CheckResource failed; resourceId given is null.")
		return false, err
	}

	var parentResourceType string
	// Check resourceType's validity
	if resourceType == common.StrSubnet {
		parentResourceType = common.StrVNet
		// continue
	} else {
		err := fmt.Errorf("invalid resource type")
		return false, err
	}

	err := common.CheckString(nsId)
	if err != nil {
		common.CBLog.Error(err)
		return false, err
	}

	err = common.CheckString(parentResourceId)
	if err != nil {
		common.CBLog.Error(err)
		return false, err
	}

	err = common.CheckString(resourceId)
	if err != nil {
		common.CBLog.Error(err)
		return false, err
	}

	fmt.Printf("[Check child resource] %s, %s, %s", resourceType, parentResourceId, resourceId)

	key := common.GenResourceKey(nsId, parentResourceType, parentResourceId)
	key += "/" + resourceType + "/" + resourceId
	//fmt.Println(key)

	keyValue, err := common.CBStore.Get(key)
	if err != nil {
		common.CBLog.Error(err)
		return false, err
	}
	if keyValue != nil {
		return true, nil
	}
	return false, nil

}

/*
func convertSpiderResourceToTumblebugResource(resourceType string, i interface{}) (interface{}, error) {
	if resourceType == "" {
		err := fmt.Errorf("CheckResource failed; resourceType given is null.")
		return nil, err
	}

	// Check resourceType's validity
	if resourceType == common.StrImage ||
		resourceType == common.StrSSHKey ||
		resourceType == common.StrSpec ||
		resourceType == common.StrVNet ||
		resourceType == common.StrSecurityGroup {
		//resourceType == "subnet" ||
		//resourceType == "publicIp" ||
		//resourceType == "vNic" {
		// continue
	} else {
		err := fmt.Errorf("invalid resource type")
		return nil, err
	}

}
*/

// https://stackoverflow.com/questions/45139954/dynamic-struct-as-parameter-golang

type ReturnValue struct {
	CustomStruct interface{}
}

type NameOnly struct {
	Name string
}

// GetNameFromStruct accepts any struct for argument, and returns
func GetNameFromStruct(u interface{}) string {
	var result = ReturnValue{CustomStruct: u}

	//fmt.Println(result)

	msg, ok := result.CustomStruct.(NameOnly)
	if ok {
		//fmt.Printf("Message1 is %s\n", msg.Name)
		return msg.Name
	} else {
		return ""
	}
}

//func createResource(nsId string, resourceType string, u interface{}) (interface{}, int, []byte, error) {

// LoadCommonResource is to register common resources from asset files (../assets/*.csv)
func LoadCommonResource() error {

	// Check 'common' namespace. Create one if not.
	commonNsId := "common"
	_, err := common.GetNs(commonNsId)
	if err != nil {
		nsReq := common.NsReq{}
		nsReq.Name = commonNsId
		nsReq.Description = "Namespace for common resources"
		_, nsErr := common.CreateNs(&nsReq)
		if nsErr != nil {
			common.CBLog.Error(nsErr)
			return nsErr
		}
	}

	// Read common specs and register spec objects
	file, fileErr := os.Open("../assets/cloudspec.csv")
	defer file.Close()
	if fileErr != nil {
		common.CBLog.Error(fileErr)
		return fileErr
	}

	rdr := csv.NewReader(bufio.NewReader(file))
	rows, _ := rdr.ReadAll()
	specReqTmp := TbSpecReq{}
	for i, row := range rows[1:] {

		// [0]connectionName, [1]cspSpecName, [2]CostPerHour
		specReqTmp.ConnectionName = row[0]
		specReqTmp.CspSpecName = row[1]
		// Give a name for spec object by combining ConnectionName and CspSpecName
		// To avoid naming-rule violation, modify the string
		specReqTmp.Name = specReqTmp.ConnectionName + "-" + specReqTmp.CspSpecName
		specReqTmp.Name = ToNamingRuleCompatible(specReqTmp.Name)

		specReqTmp.Description = "Common Spec Resource"

		fmt.Printf("[%d] Register Common Spec\n", i)
		common.PrintJsonPretty(specReqTmp)

		// Register Spec object
		_, err := RegisterSpecWithCspSpecName(commonNsId, &specReqTmp)
		if err != nil {
			common.CBLog.Error(err)
			// If already exist, error will occur
			// Even if error, do not return here to update information
			// return err
		}
		specObjId := specReqTmp.Name

		// Update registered Spec object with Cost info
		costPerHour, err := strconv.ParseFloat(strings.ReplaceAll(row[2], " ", ""), 32)
		if err != nil {
			common.CBLog.Error(err)
			// If already exist, error will occur
			// Even if error, do not return here to update information
			// return err
		}
		costPerHour32 := float32(costPerHour)
		specUpdateRequest := TbSpecInfo{CostPerHour: costPerHour32}

		updatedSpecInfo, err := UpdateSpec(commonNsId, specObjId, specUpdateRequest)
		if err != nil {
			common.CBLog.Error(err)
			// If already exist, error will occur
			// Even if error, do not return here to update information
			// return err
		}
		fmt.Printf("[%d] Registered Common Spec\n", i)
		common.PrintJsonPretty(updatedSpecInfo)

	}

	// Read common specs and register spec objects
	file, fileErr = os.Open("../assets/cloudimage.csv")
	defer file.Close()
	if fileErr != nil {
		common.CBLog.Error(fileErr)
		return fileErr
	}

	rdr = csv.NewReader(bufio.NewReader(file))
	rows, _ = rdr.ReadAll()
	imageReqTmp := TbImageReq{}
	for i, row := range rows[1:] {

		// row0: ProviderName
		// row1: connectionName
		// row2: cspImageId
		// row3: OsType
		imageReqTmp.ConnectionName = row[1]
		imageReqTmp.CspImageId = row[2]
		osType := strings.ReplaceAll(row[3], " ", "")
		// Give a name for spec object by combining ConnectionName and OsType
		// To avoid naming-rule violation, modify the string
		imageReqTmp.Name = imageReqTmp.ConnectionName + "-" + osType
		imageReqTmp.Name = ToNamingRuleCompatible(imageReqTmp.Name)
		imageReqTmp.Description = "Common Image Resource"

		fmt.Printf("[%d] Register Common Image\n", i)
		common.PrintJsonPretty(imageReqTmp)

		// Register Spec object
		_, err := RegisterImageWithId(commonNsId, &imageReqTmp)
		if err != nil {
			common.CBLog.Error(err)
			// If already exist, error will occur
			// Even if error, do not return here to update information
			//return err
		}

		// Update registered image object with OsType info
		imageObjId := imageReqTmp.Name

		imageUpdateRequest := TbImageInfo{GuestOS: osType}

		updatedImageInfo, err := UpdateImage(commonNsId, imageObjId, imageUpdateRequest)
		if err != nil {
			common.CBLog.Error(err)
			//return err
		}
		fmt.Printf("[%d] Registered Common Image\n", i)
		common.PrintJsonPretty(updatedImageInfo)
	}

	return nil
}

// LoadDefaultResource is to register default resource from asset files (../assets/*.csv)
func LoadDefaultResource(nsId string, resType string, connectionName string) error {

	// Check 'nsId' namespace.
	_, err := common.GetNs(nsId)
	if err != nil {
		common.CBLog.Error(err)
		return err
	}

	var resList []string
	if resType == "all" {
		resList = append(resList, "vnet")
		resList = append(resList, "sshkey")
		resList = append(resList, "sg")
	} else {
		resList = append(resList, strings.ToLower(resType))
	}

	// Read default resources from file and create objects
	// HEADER: ProviderName, CONN_CONFIG, RegionName, RegionLocation, DriverLibFileName, DriverName
	file, fileErr := os.Open("../assets/cloudconnection.csv")
	defer file.Close()
	if fileErr != nil {
		common.CBLog.Error(fileErr)
		return fileErr
	}

	rdr := csv.NewReader(bufio.NewReader(file))
	rows, err := rdr.ReadAll()
	if err != nil {
		common.CBLog.Error(err)
		return err
	}

	for i, row := range rows[1:] {
		if connectionName != "" {
			// find only given connectionName (if not skip)
			if connectionName != row[1] {
				continue
			}
			fmt.Println("Found a line for the connectionName from file: " + row[1])
		}

		connectionName := row[1]
		//resourceName := connectionName
		// Default resource name has this pattern (nsId + "-default-" + connectionName)
		resourceName := nsId + common.StrDefaultResourceName + connectionName
		description := "Generated Default Resource"

		for _, resType := range resList {
			if resType == "vnet" {
				fmt.Println("vnet")

				reqTmp := TbVNetReq{}
				reqTmp.ConnectionName = connectionName
				reqTmp.Name = resourceName
				reqTmp.Description = description

				// set isolated private address space for each cloud region (192.168.xxx.0/24)
				reqTmp.CidrBlock = "192.168." + strconv.Itoa(i+1) + ".0/24"

				// subnet := SpiderSubnetReqInfo{Name: reqTmp.Name, IPv4_CIDR: reqTmp.CidrBlock}
				subnet := TbSubnetReq{Name: reqTmp.Name, IPv4_CIDR: reqTmp.CidrBlock}
				reqTmp.SubnetInfoList = append(reqTmp.SubnetInfoList, subnet)

				common.PrintJsonPretty(reqTmp)

				resultInfo, err := CreateVNet(nsId, &reqTmp)
				if err != nil {
					common.CBLog.Error(err)
					// If already exist, error will occur
					// Even if error, do not return here to update information
					// return err
				}
				fmt.Printf("[%d] Registered Default vNet\n", i)
				common.PrintJsonPretty(resultInfo)
			} else if resType == "sg" || resType == "securitygroup" {
				fmt.Println("sg")

				reqTmp := TbSecurityGroupReq{}

				reqTmp.ConnectionName = connectionName
				reqTmp.Name = resourceName
				reqTmp.Description = description

				reqTmp.VNetId = resourceName

				// open all firewall for default securityGroup
				rule := SpiderSecurityRuleInfo{FromPort: "1", ToPort: "65535", IPProtocol: "tcp", Direction: "inbound", CIDR: "0.0.0.0/0"}
				var ruleList []SpiderSecurityRuleInfo
				ruleList = append(ruleList, rule)
				rule = SpiderSecurityRuleInfo{FromPort: "1", ToPort: "65535", IPProtocol: "udp", Direction: "inbound", CIDR: "0.0.0.0/0"}
				ruleList = append(ruleList, rule)
				rule = SpiderSecurityRuleInfo{FromPort: "-1", ToPort: "-1", IPProtocol: "icmp", Direction: "inbound", CIDR: "0.0.0.0/0"}
				ruleList = append(ruleList, rule)
				common.PrintJsonPretty(ruleList)
				reqTmp.FirewallRules = &ruleList

				common.PrintJsonPretty(reqTmp)

				resultInfo, err := CreateSecurityGroup(nsId, &reqTmp)
				if err != nil {
					common.CBLog.Error(err)
					// If already exist, error will occur
					// Even if error, do not return here to update information
					// return err
				}
				fmt.Printf("[%d] Registered Default SecurityGroup\n", i)
				common.PrintJsonPretty(resultInfo)

			} else if resType == "sshkey" {
				fmt.Println("sshkey")

				reqTmp := TbSshKeyReq{}

				reqTmp.ConnectionName = connectionName
				reqTmp.Name = resourceName
				reqTmp.Description = description

				common.PrintJsonPretty(reqTmp)

				resultInfo, err := CreateSshKey(nsId, &reqTmp)
				if err != nil {
					common.CBLog.Error(err)
					// If already exist, error will occur
					// Even if error, do not return here to update information
					// return err
				}
				fmt.Printf("[%d] Registered Default SSHKey\n", i)
				common.PrintJsonPretty(resultInfo)
			} else {
				return errors.New("Not valid option (provide sg, sshkey, vnet, or all)")
			}
		}

		if connectionName != "" {
			// After finish handling line for the connectionName, break
			if connectionName == row[1] {
				fmt.Println("Handled for the connectionName from file: " + row[1])
				break
			}
		}
	}
	return nil
}

// ToNamingRuleCompatible func is a tool to replace string for name to make the name follow naming convention
func ToNamingRuleCompatible(rawName string) string {
	rawName = strings.ReplaceAll(rawName, " ", "-")
	rawName = strings.ReplaceAll(rawName, ".", "-")
	rawName = strings.ReplaceAll(rawName, "_", "-")
	rawName = strings.ToLower(rawName)
	return rawName
}
