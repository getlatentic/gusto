package runner

import (
	"net"
	"testing"

	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/job"
)

// A job run in the server's own process is given the proxy, and it is the proxy that has to answer, not the model address the
// server itself would use.
func TestAHarnessNeedingAModelIsAdmittedWhenTheAddressItIsGivenAnswers(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	t.Setenv("GUSTO_MODEL_URL", "http://127.0.0.1:1/v1")
	adapter := harness.Adapter{Name: "local", Program: "sh", Needs: harness.NeedsLocal}
	given := harness.Env{ModelURL: "http://" + listener.Addr().String() + "/model/v1"}
	if err := admit(job.Spec{Harness: "local"}, adapter, given); err != nil {
		t.Errorf("the address the harness is given answers, and it was refused: %v", err)
	}
	if err := admit(job.Spec{Harness: "local"}, adapter, harness.Env{}); err == nil {
		t.Error("with no address given, the server's own was not asked")
	}
}
