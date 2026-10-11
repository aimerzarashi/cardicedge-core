package importconsumer

// generated.go は exclude-files のデフォルトパターンに該当するため、
// エイリアスがあっても検知されない
import gen "importalias/importtarget"

func UseGenerated() string {
	return gen.Name
}
