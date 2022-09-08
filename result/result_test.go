package result

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResult(t *testing.T) {
	resultA := OK("good")
	resultB := Err[string](fmt.Errorf("bad"))

	valueA, errA := resultA.Unwrap()
	require.NoError(t, errA)
	require.Equal(t, valueA, "good")
	fmt.Printf("valueA: %s\n", valueA)

	_, errB := resultB.Unwrap()
	require.Error(t, errB)
	require.Equal(t, errB.Error(), "bad")
	fmt.Printf("errB: %s\n", errB)
}
