// Copyright © 2025 univero. All rights reserved.
// Licensed under the GNU Affero General Public License v3 (AGPL-3.0).
// license that can be found in the LICENSE file.

package app

import (
	"errors"
)

var (
	// End 用于流式中标识输出完成
	End = errors.New("[app] no more")
	// NoFactory 表示对应的平台没有实现
	NoFactory = errors.New("[app] no app factory")
)
