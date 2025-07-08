package httpx

import "github.com/xh-polaris/psych-idl/kitex_gen/basic"

func succeed() *basic.Response {
	return &basic.Response{
		Code: 0,
		Msg:  "success",
	}
}
