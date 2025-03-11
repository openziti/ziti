/*
	Copyright NetFoundry Inc.

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

package env

var routers []ApiRouter

type AddRouterFunc func(ae *AppEnv)

type ApiRouter interface {
	Register(ae *AppEnv)
}

type ApiRouterMiddleware interface {
	AddMiddleware(ae *AppEnv)
}

func AddRouter(rf ApiRouter) {
	routers = append(routers, rf)
}

func GetRouters() []ApiRouter {
	return routers
}
