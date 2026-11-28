package app

import (
	"testing"

	"github.com/Oceanway-AI/oceanway-platform/platformclient"
)

func TestStudioAgentGenerationDeclaresTaskFundingReceipt(t *testing.T) {
	envelope := studioExecutionEnvelope{RequestID: "original", Body: []byte(`{"model":"image-model","n":1}`), Consumption: &platformclient.MediaConsumptionConstraints{WalletMaxPoints: "25"}}
	command := studioAgentGenerationCommand(envelope)
	if command.ReceiptContract != platformclient.MediaTaskFundingReceiptV1 {
		t.Fatal("Agent submission must declare its supported Task receipt")
	}
	if string(command.NativeRequest) != string(envelope.Body) || command.RequestID != envelope.RequestID || command.Consumption.WalletMaxPoints != "25" {
		t.Fatal("receipt declaration changed the original generation authority")
	}
}
