package utils

import (
	"fmt"

	borweinconsts "github.com/kubewharf/katalyst-core/pkg/agent/sysadvisor/plugin/inference/models/borwein/consts"
)

func GetInferenceResultKey(modelName string) string {
	// legacy model name compatible
	if modelName == borweinconsts.ModelNameBorwein {
		return modelName
	}

	return fmt.Sprintf("%s/%s", borweinconsts.ModelNameBorwein, modelName)
}
