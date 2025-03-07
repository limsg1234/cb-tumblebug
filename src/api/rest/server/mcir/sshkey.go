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

// Package mcir is to handle REST API for mcir
package mcir

import (
	"fmt"
	"net/http"

	"github.com/cloud-barista/cb-tumblebug/src/core/common"
	"github.com/cloud-barista/cb-tumblebug/src/core/mcir"
	"github.com/labstack/echo/v4"
)

// RestPostSshKey godoc
// @Summary Create SSH Key
// @Description Create SSH Key
// @Tags [MCIR] Access key management
// @Accept  json
// @Produce  json
// @Param nsId path string true "Namespace ID"
// @Param sshKeyInfo body mcir.TbSshKeyReq true "Details for an SSH Key object"
// @Success 200 {object} mcir.TbSshKeyInfo
// @Failure 404 {object} common.SimpleMsg
// @Failure 500 {object} common.SimpleMsg
// @Router /ns/{nsId}/resources/sshKey [post]
func RestPostSshKey(c echo.Context) error {

	nsId := c.Param("nsId")

	u := &mcir.TbSshKeyReq{}
	if err := c.Bind(u); err != nil {
		return err
	}

	fmt.Println("[POST SshKey")
	//fmt.Println("[Creating SshKey]")
	//content, responseCode, _, err := CreateSshKey(nsId, u)
	content, err := mcir.CreateSshKey(nsId, u)
	if err != nil {
		common.CBLog.Error(err)
		mapA := map[string]string{"message": err.Error()}
		return c.JSON(http.StatusInternalServerError, &mapA)
	}
	return c.JSON(http.StatusCreated, content)
}

/* function RestPutSshKey not yet implemented
// RestPutSshKey godoc
// @Summary Update SSH Key
// @Description Update SSH Key
// @Tags [MCIR] Access key management
// @Accept  json
// @Produce  json
// @Param sshKeyInfo body mcir.TbSshKeyInfo true "Details for an SSH Key object"
// @Success 200 {object} mcir.TbSshKeyInfo
// @Failure 404 {object} common.SimpleMsg
// @Failure 500 {object} common.SimpleMsg
// @Router /ns/{nsId}/resources/sshKey/{sshKeyId} [put]
*/
func RestPutSshKey(c echo.Context) error {
	//nsId := c.Param("nsId")

	return nil
}

// RestGetSshKey godoc
// @Summary Get SSH Key
// @Description Get SSH Key
// @Tags [MCIR] Access key management
// @Accept  json
// @Produce  json
// @Param nsId path string true "Namespace ID"
// @Param sshKeyId path string true "SSH Key ID"
// @Success 200 {object} mcir.TbSshKeyInfo
// @Failure 404 {object} common.SimpleMsg
// @Failure 500 {object} common.SimpleMsg
// @Router /ns/{nsId}/resources/sshKey/{sshKeyId} [get]
func RestGetSshKey(c echo.Context) error {
	// This is a dummy function for Swagger.
	return nil
}

// Response struct for RestGetAllSshKey
type RestGetAllSshKeyResponse struct {
	SshKey []mcir.TbSshKeyInfo `json:"sshKey"`
}

// RestGetAllSshKey godoc
// @Summary List all SSH Keys or SSH Keys' ID
// @Description List all SSH Keys or SSH Keys' ID
// @Tags [MCIR] Access key management
// @Accept  json
// @Produce  json
// @Param nsId path string true "Namespace ID"
// @Param option query string false "Option" Enums(id)
// @Success 200 {object} JSONResult{[DEFAULT]=RestGetAllSshKeyResponse,[ID]=common.IdList} "Different return structures by the given option param"
// @Failure 404 {object} common.SimpleMsg
// @Failure 500 {object} common.SimpleMsg
// @Router /ns/{nsId}/resources/sshKey [get]
func RestGetAllSshKey(c echo.Context) error {
	// This is a dummy function for Swagger.
	return nil
}

// RestDelSshKey godoc
// @Summary Delete SSH Key
// @Description Delete SSH Key
// @Tags [MCIR] Access key management
// @Accept  json
// @Produce  json
// @Param nsId path string true "Namespace ID"
// @Param sshKeyId path string true "SSH Key ID"
// @Success 200 {object} common.SimpleMsg
// @Failure 404 {object} common.SimpleMsg
// @Router /ns/{nsId}/resources/sshKey/{sshKeyId} [delete]
func RestDelSshKey(c echo.Context) error {
	// This is a dummy function for Swagger.
	return nil
}

// RestDelAllSshKey godoc
// @Summary Delete all SSH Keys
// @Description Delete all SSH Keys
// @Tags [MCIR] Access key management
// @Accept  json
// @Produce  json
// @Param nsId path string true "Namespace ID"
// @Success 200 {object} common.SimpleMsg
// @Failure 404 {object} common.SimpleMsg
// @Router /ns/{nsId}/resources/sshKey [delete]
func RestDelAllSshKey(c echo.Context) error {
	// This is a dummy function for Swagger.
	return nil
}
