package indexes

import (
	"fmt"

	wfv1 "github.com/argoproj/argo-workflows/v4/pkg/apis/workflow/v1alpha1"
	unwf "github.com/argoproj/argo-workflows/v4/util/unstructured/workflow"
)

func ConditionsIndexFunc(obj any) ([]string, error) {
	var values []string
	for _, x := range unwf.GetConditions(obj) {
		values = append(values, ConditionValue(x))
	}
	return values, nil
}

func ConditionValue(x wfv1.Condition) string {
	return fmt.Sprintf("%s/%s", x.Type, x.Status)
}
