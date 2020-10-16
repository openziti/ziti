# Prerequisites

1. Install the protoc binary from: https://github.com/protocolbuffers/protobuf/releases
2. Install the protoc plugin for Go ```go get -u github.com/golang/protobuf/protoc-gen-go```
3. Ensure ```protoc``` is on your path.
4. Ensure your Go bin directory is on your path


# Generate Go Code

Two options, run the command manually or use `go generate`

## Go Generate

1. Navigate to the root project
2. run `go generate edge/pb/edge_ctrl_pb/...`

Note: Running a naked `go generate` will trigger all `go:generate` tags in the project, which you most likely do not want

## Manually

1. Navigate to the project root
2. Run: ```protoc -I ./pb/edge_ctrl_pb/ ./pb/edge_ctrl_pb/edge_ctrl.proto --go_out=./pb/edge_ctrl_pb```
