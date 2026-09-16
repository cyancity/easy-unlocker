package protocol

import "testing"

func baseRequest() Request {
	return Request{Item: "OPENAI_KEY", Mode: ModeWrite, Purpose: "冒烟", TTL: 60, Target: "/tmp/x"}
}

func TestValidateDelivery(t *testing.T) {
	cases := []struct {
		name     string
		delivery string
		wantErr  bool
	}{
		{"空串等价落盘（老 CLI）", "", false},
		{"file", DeliveryFile, false},
		{"ephemeral", DeliveryEphemeral, false},
		{"不认识的值", "stdout", true},
		{"大小写敏感", "FILE", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := baseRequest()
			request.Delivery = tc.delivery
			err := request.Validate(3600)
			if tc.wantErr && err == nil {
				t.Fatal("want error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// ephemeral 不落盘，不需要 target，但 item/purpose/ttl 仍然必填。
func TestValidateEphemeralWithoutTarget(t *testing.T) {
	request := baseRequest()
	request.Target = ""
	request.Delivery = DeliveryEphemeral
	if err := request.Validate(3600); err != nil {
		t.Fatalf("ephemeral 不该要求 target: %v", err)
	}
	request.Purpose = ""
	if err := request.Validate(3600); err == nil {
		t.Fatal("purpose 仍应必填")
	}
}
