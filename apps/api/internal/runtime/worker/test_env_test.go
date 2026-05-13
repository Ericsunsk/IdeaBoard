package worker

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	_ = os.Setenv("API_OUTBOUND_ALLOW_HOSTS", "localhost,127.0.0.1,::1")
	os.Exit(m.Run())
}
