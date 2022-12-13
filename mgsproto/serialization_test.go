package mgsproto

import (
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSwitcheroo(t *testing.T) {
	uuid1 := uuid.MustParse("a3de282f-478b-a6e6-812e-f34f87bd449e")
	uuid2 := uuid.MustParse("812ef34f-87bd-449e-a3de-282f478ba6e6")

	assert.Equal(t, uuid1.String(), uuidSwitcheroo(uuid2).String())
	assert.Equal(t, uuid2.String(), uuidSwitcheroo(uuid1).String())
}
