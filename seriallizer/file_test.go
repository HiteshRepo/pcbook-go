package seriallizer

import (
	"testing"

	"github.com/hiteshrepo/pcbook/pb"
	"github.com/hiteshrepo/pcbook/sample"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestWriteProtobufToBinaryFile(t *testing.T) {
	t.Parallel()

	binaryFile := "../tmp/laptop-write.bin"

	laptop1 := sample.NewLaptop()
	err := WriteProtobufToBinaryFile(laptop1, binaryFile)

	require.NoError(t, err)
}

func TestReadProtobufFromBinaryFile(t *testing.T) {
	t.Parallel()

	binaryFile := "../tmp/laptop-read.bin"

	laptop1 := sample.NewLaptop()
	WriteProtobufToBinaryFile(laptop1, binaryFile)

	laptop2 := &pb.Laptop{}
	err := ReadProtobufFromBinaryFile(laptop2, binaryFile)

	require.NoError(t, err)
	require.True(t, proto.Equal(laptop1, laptop2))
}

func TestWriteProtobufToJSONFile(t *testing.T) {
	t.Parallel()

	jsonFile := "../tmp/laptop.json"

	laptop1 := sample.NewLaptop()
	err := WriteProtobufToJSONFile(laptop1, jsonFile)

	require.NoError(t, err)
}
