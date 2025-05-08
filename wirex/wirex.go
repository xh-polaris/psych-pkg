package wirex

import "github.com/google/wire"

// NewWireSet 创造一个默认的wire依赖注入集合, 使用方式NewWireSet[类型, 接口]()
// 对应需要依赖字段注入的则需要自行编写
func NewWireSet[S any, I any]() wire.ProviderSet {
	return wire.NewSet(
		wire.Struct(new(S), "*"),
		wire.Bind(new(I), new(*S)),
	)
}
