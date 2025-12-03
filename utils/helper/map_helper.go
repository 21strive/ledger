package helper

import (
	"encoding/json"
	"reflect"
	"time"

	"github.com/google/uuid"

	maps "github.com/mitchellh/mapstructure"
)

func DecodeMapType(input interface{}, output interface{}) error {
	config := &maps.DecoderConfig{
		Metadata:   nil,
		Result:     output,
		TagName:    "json",
		DecodeHook: maps.ComposeDecodeHookFunc(toTimeHookFunc(), toUUIDHookFunc()),
	}

	decoder, err := maps.NewDecoder(config)
	if err != nil {
		return err
	}

	return decoder.Decode(input)
}

func toTimeHookFunc() maps.DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data interface{}) (interface{}, error) {
		if t != reflect.TypeOf(time.Time{}) {
			return data, nil
		}

		switch f.Kind() {
		case reflect.String:
			return time.Parse(time.RFC3339, data.(string))
		case reflect.Float64:
			return time.Unix(0, int64(data.(float64))*int64(time.Millisecond)), nil
		case reflect.Int64:
			return time.Unix(0, data.(int64)*int64(time.Millisecond)), nil
		default:
			return data, nil
		}
		// Convert it by parsing
	}
}

func toUUIDHookFunc() maps.DecodeHookFunc {
	return func(
		f reflect.Type,
		t reflect.Type,
		data interface{}) (interface{}, error) {
		if t != reflect.TypeOf(uuid.UUID{}) {
			return data, nil
		}

		switch f.Kind() {
		case reflect.String:
			uuid, err := uuid.Parse(data.(string))
			if err != nil {
				return data, nil
			}
			return uuid, nil
		default:
			return data, nil
		}
		// Convert it by parsing
	}
}

func MergeMaps(maps ...map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}

	return result
}

func MakeSortOrder(order int) string {
	if order == -1 {
		return "DESC"
	}

	return "ASC"
}

func StructToMap(input interface{}) map[string]interface{} {
	requestMap := make(map[string]interface{})
	jsonRequest, _ := json.Marshal(input)
	json.Unmarshal(jsonRequest, &requestMap)
	return requestMap
}

func SliceStructToMapSlice(input interface{}) []map[string]interface{} {
	requestMap := []map[string]interface{}{}
	jsonRequest, _ := json.Marshal(input)
	json.Unmarshal(jsonRequest, &requestMap)
	return requestMap
}

func Contains[T comparable](s []T, e T) bool {
	for _, v := range s {
		if v == e {
			return true
		}
	}
	return false
}

// ArrayContains (Array, Any) find is number in slice
func ArrayContains(slice interface{}, need interface{}) bool {
	arr := reflect.ValueOf(slice)
	switch arr.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < arr.Len(); i++ {
			if reflect.DeepEqual(need, arr.Index(i).Interface()) {
				return true
			}
		}
	case reflect.Map:
		for _, m := range arr.MapKeys() {
			if reflect.DeepEqual(need, arr.MapIndex(m).Interface()) {
				return true
			}
		}
	}

	return false
}
