/*
	Copyright 2019 Netfoundry, Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package edge

import (
	"encoding/json"
	"io"
)

type Session struct {
	Id    string `json:"id"`
	Token string `json:"token"`
	//Tags  []string `json:"tags"`
}

type Gateway struct {
	Name     string `json:"name"`
	Hostname string `json:"hostname"`
	Urls     map[string]string
}

type NetworkSession struct {
	Id        string    `json:"id"`
	Token     string    `json:"token"`
	SessionId string    `json:"sessionId"`
	Gateways  []Gateway `json:"gateways"`
}

type Service struct {
	Name string `json:"name"`
	Id   string `json:"id"`
	Dns  struct {
		Hostname string `json:"hostname"`
		Port     int    `json:"port"`
	} `json:"dns"`
	Hostable bool              `json:"hostable"`
	Tags     map[string]string `json:"tags"`
}

type EdgeControllerApiError struct {
	Error struct {
		Args struct {
			URLVars struct {
			} `json:"urlVars"`
		} `json:"args"`
		Cause struct {
			Message    string `json:"message"`
			FieldName  string `json:"fieldName"`
			FieldValue string `json:"fieldValue"`
		} `json:"cause"`
		CauseMessage string `json:"causeMessage"`
		Code         string `json:"code"`
		Message      string `json:"message"`
		RequestID    string `json:"requestId"`
	} `json:"error"`
	Meta struct {
		APIEnrolmentVersion string `json:"apiEnrolmentVersion"`
		APIVersion          string `json:"apiVersion"`
	} `json:"meta"`
}

type apiResponse struct {
	Data interface{}          `json:"data"`
	Meta *ApiResponseMetadata `json:"meta"`
}

type ApiResponseMetadata struct {
	FilterableFields []string `json:"filterableFields"`
	Pagination       *struct {
		Offset     int `json:"offset"`
		Limit      int `json:"limit"`
		TotalCount int `json:"totalCount"`
	} `json:"pagination"`
}

func ApiResponseDecode(data interface{}, resp io.Reader) (*ApiResponseMetadata, error) {
	apiR := &apiResponse{
		Data: data,
	}
	if err := json.NewDecoder(resp).Decode(apiR); err != nil {
		return nil, err
	}

	return apiR.Meta, nil
}
