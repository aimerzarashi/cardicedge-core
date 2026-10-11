package importconsumer

import (
	alias "importalias/importtarget" // want `import alias for "importalias/importtarget" is prohibited\. Use the package's canonical name\.`
)

func UseAliased() string {
	return alias.Name
}
