package servicelog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewListCmd(t *testing.T) {
	cmd := newListCmd()
	assert.NotNil(t, cmd)
	assert.Equal(t, "list [flags] [options] cluster-identifier", cmd.Use)
	assert.Equal(t, "Get service logs for a given cluster identifier.", cmd.Short)
	assert.Contains(t, cmd.Long, "Get service logs for a given cluster identifier.")

	err := cmd.Args(cmd, []string{"test-cluster"})
	assert.NoError(t, err)
	err = cmd.Args(cmd, []string{})
	assert.Error(t, err)

	allMessagesFlag := cmd.Flags().Lookup(AllMessagesFlag)
	assert.NotNil(t, allMessagesFlag)
	assert.Equal(t, "A", allMessagesFlag.Shorthand)
	assert.Equal(t, "Toggle if we should see all of the messages or only SRE-P specific ones", allMessagesFlag.Usage)
	assert.False(t, allMessagesFlag.Value.String() == "true")

	internalFlag := cmd.Flags().Lookup(InternalFlag)
	assert.NotNil(t, internalFlag)
	assert.Equal(t, "i", internalFlag.Shorthand)
	assert.Equal(t, "Toggle if we should see internal messages", internalFlag.Usage)
	assert.False(t, internalFlag.Value.String() == "true")
}

func TestListCmdOptions(t *testing.T) {
	opts := &listCmdOptions{
		allMessages: true,
		internal:    true,
	}
	assert.True(t, opts.allMessages)
	assert.True(t, opts.internal)
}

func Test_listServiceLogs(t *testing.T) {
	type args struct {
		clusterID string
		opts      *listCmdOptions
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := listServiceLogs(tt.args.clusterID, tt.args.opts); (err != nil) != tt.wantErr {
				t.Errorf("listServiceLogs() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
