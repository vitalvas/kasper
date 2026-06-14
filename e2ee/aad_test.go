package e2ee

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRequestAAD(t *testing.T) {
	got := requestAAD(`"k";aead="AES-256-GCM"`)
	assert.Equal(t, `e2ee/v1:req "k";aead="AES-256-GCM"`, string(got))
}

func TestResponseAAD(t *testing.T) {
	got := responseAAD(`"k";ts=1`, `"k";ts=2`)
	assert.Equal(t, `e2ee/v1:res "k";ts=1 "k";ts=2`, string(got))
}
