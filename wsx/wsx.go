package wsx

import "errors"

func IsNormal(err error) bool {
	return errors.Is(err, NormalCloseErr)
}
