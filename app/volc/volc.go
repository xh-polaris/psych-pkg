package volc

import (
	"github.com/bytedance/gopkg/lang/fastrand"
	"strconv"
	"strings"
	"time"
)

// genLogID 生成日志ID
func genLogID() string {
	const (
		maxRandNum = 1<<24 - 1<<20
		length     = 53
		version    = "02"
		localIP    = "00000000000000000000000000000000"
	)
	ts := uint64(time.Now().UnixNano() / int64(time.Millisecond))
	r := uint64(fastrand.Uint32n(maxRandNum) + 1<<20)
	var sb strings.Builder
	sb.Grow(length)
	sb.WriteString(version)
	sb.WriteString(strconv.FormatUint(ts, 10))
	sb.WriteString(localIP)
	sb.WriteString(strconv.FormatUint(r, 16))
	return sb.String()
}
