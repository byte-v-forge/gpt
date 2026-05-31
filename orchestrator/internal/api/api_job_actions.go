package api

import (
	"google.golang.org/protobuf/proto"

	"orchestrator/internal/jobdata"
	"orchestrator/pb"
)

func jobDataMessage(value proto.Message) *pb.JobData {
	return jobdata.Message(value)
}
