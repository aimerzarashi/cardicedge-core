package importconsumer

import "importalias/importtarget"

// 正式名でのimportはOK
func UseCanonical() string {
	return importtarget.Name
}
