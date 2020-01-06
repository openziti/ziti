#Prerequisites

1) Install the protoc binary from: https://github.com/protocolbuffers/protobuf/releases
1) Install the protoc plugin for Go ```go get -u github.com/golang/protobuf/protoc-gen-go```
1) Ensure ```protoc``` is on your path.
1) Ensure your Go bin directory is on your path


#Generate Go Code
1) Navigate to the ziti project root
1) Run: ```protoc -I ./edge/pb/edge_ctrl_pb/ ./edge/pb/edge_ctrl_pb/edge_ctrl.proto --go_out=edge/pb/edge_ctrl_pb```
