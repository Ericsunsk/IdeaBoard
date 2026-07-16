package credits

import "testing"

func TestCommandsInSQLTxRejectsNilTransaction(t *testing.T) {
	commands, err := CommandsInSQLTx(nil)
	if err == nil || commands != nil {
		t.Fatalf("commands=%v error=%v, want explicit nil transaction error", commands, err)
	}
}
