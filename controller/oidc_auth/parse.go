package oidc_auth

import (
	"encoding/json"
	"fmt"
	"github.com/openziti/edge-api/rest_model"
	"github.com/openziti/foundation/v2/errorz"
	"io"
	"net/http"
	"reflect"
	"strings"
	"unicode/utf8"
)

type Totp struct {
	AuthRequestBody
	Code string `json:"code"`
}

type updbCreds struct {
	rest_model.Authenticate
	AuthRequestBody
}

func (u *updbCreds) Translate(in string, paths ...string) (string, bool) {
	if len(paths) > 0 {
		last := paths[len(paths)-1:][0]

		switch last {
		case "EnvInfo":
			return "env" + upperCaseInitial(in), true
		case "SdkInfo":
			return "sdk" + upperCaseInitial(in), true
		}
	}
	return "", false
}

type AuthRequestBody struct {
	AuthRequestId string `json:"id"`
}

func (a *AuthRequestBody) SetAuthRequestId(id string) {
	a.AuthRequestId = id
}

func (a *AuthRequestBody) GetAuthRequestId() string {
	return a.AuthRequestId
}

var _ AuthRequestIdHolder = (*AuthRequestBody)(nil)

type AuthRequestIdHolder interface {
	SetAuthRequestId(string)
	GetAuthRequestId() string
}

type FieldTranslator interface {
	Translate(string, ...string) (string, bool)
}

func MapToStruct(m map[string][]string, dst interface{}) error {
	translator, _ := dst.(FieldTranslator)
	return mapToStruct(0, m, dst, translator)
}

func mapToStruct(depth int, src map[string][]string, dst interface{}, translator FieldTranslator, paths ...string) error {
	if paths == nil {
		paths = []string{}
	}

	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("expected a pointer to a struct")
	}

	rv = rv.Elem()
	rt := rv.Type()

	for i := 0; i < rv.NumField(); i++ {
		field := rv.Field(i)
		fieldType := rt.Field(i)
		fieldName := fieldType.Name

		tagValue := fieldType.Tag.Get("json")
		tagVals := strings.Split(tagValue, ",")

		if len(tagVals) == 0 {
			tagVals = []string{""}
		}
		if tagVals[0] == "-" {
			continue
		}

		if field.Kind() == reflect.Struct {
			fieldPtr := field.Addr().Interface()

			newPaths := make([]string, len(paths))
			copy(newPaths, paths)

			if depth > 0 {
				newPaths = append(newPaths, fieldName)
			}

			err := mapToStruct(depth+1, src, fieldPtr, translator, newPaths...)
			if err != nil {
				return err
			}
			continue
		} else if field.Kind() == reflect.Ptr && field.Type().Elem().Kind() == reflect.Struct {
			var fieldPtr interface{}

			newPaths := make([]string, len(paths))
			copy(newPaths, paths)

			if depth > 0 {
				newPaths = append(newPaths, fieldName)
			}

			if !field.IsNil() {
				fieldPtr = field.Interface()
			} else {
				// Initialize the nil pointer to a new struct before proceeding
				newStruct := reflect.New(field.Type().Elem())
				field.Set(newStruct)
				fieldPtr = newStruct.Interface()
			}

			err := mapToStruct(depth+1, src, fieldPtr, translator, newPaths...)
			if err != nil {
				return err
			}
			continue
		}

		var mapValue []string
		var ok bool

		if tagVals[0] != "" {
			if translator != nil {
				translation, translatedOk := translator.Translate(tagVals[0], paths...)
				if translatedOk {
					mapValue, ok = src[translation]
				} else {
					mapValue, ok = src[tagVals[0]]
				}
			} else {
				mapValue, ok = src[tagVals[0]]
			}
		}
		if !ok {
			mapValue, ok = src[fieldType.Name]
		}

		if !ok || len(mapValue) == 0 {
			continue
		}

		switch field.Kind() {
		case reflect.String:
			if len(mapValue) > 0 {
				field.SetString(mapValue[0])
			}
		case reflect.Slice:
			if fieldType.Type.Elem().Kind() == reflect.String {
				field.Set(reflect.ValueOf(mapValue))
			}
		default:
			panic("unhandled default case")
		}
	}

	return nil
}

func parsePayload(r *http.Request, out AuthRequestIdHolder) error {
	contentType, err := negotiateBodyContentType(r)

	if err != nil {
		return err
	}

	if contentType == FormContentType {
		err := r.ParseForm()
		if err != nil {
			return fmt.Errorf("cannot parse form: %s", err)
		}

		err = MapToStruct(r.Form, out)

		if err != nil {
			return err
		}

		return nil

	} else if contentType == JsonContentType {
		body, err := io.ReadAll(r.Body)

		if err != nil {
			return err
		}

		err = json.Unmarshal(body, out)

		if err != nil {
			return err
		}
	} else {
		return &errorz.ApiError{
			Code:        "UNSUPPORTED_MEDIA_TYPE",
			Message:     fmt.Sprintf("the content type: %s, is not supported (supported: %s, %s)", contentType, FormContentType, JsonContentType),
			Status:      http.StatusUnsupportedMediaType,
			Cause:       nil,
			AppendCause: false,
		}
	}

	if out.GetAuthRequestId() == "" {
		out.SetAuthRequestId(r.URL.Query().Get("id"))
	}

	return nil
}

func upperCaseInitial(in string) string {
	if in != "" {
		r, size := utf8.DecodeRuneInString(in)
		return strings.ToUpper(string(r)) + in[size:]
	}

	return ""
}
