package jobdata

import (
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"orchestrator/internal/protowrap"
	"orchestrator/pb"
)

func Message(value proto.Message) *pb.JobData {
	if value == nil {
		return nil
	}
	if typed, ok := value.(*pb.JobData); ok {
		return typed
	}
	out := &pb.JobData{}
	if !protowrap.SetMessage(out, value) {
		return nil
	}
	return out
}

func FromJSON(raw string) *pb.JobData {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	out := &pb.JobData{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal([]byte(raw), out); err != nil {
		return nil
	}
	if Unwrap(out) == nil {
		return nil
	}
	return out
}

func Unwrap(data *pb.JobData) proto.Message {
	return protowrap.FirstMessage(data)
}
