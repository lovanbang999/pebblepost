package grpcclient

import (
	"encoding/json"
	"fmt"

	"github.com/jhump/protoreflect/desc"
	"google.golang.org/protobuf/types/descriptorpb"
)

// GenerateSampleJSON generates a sample JSON string for a given protobuf message descriptor.
func GenerateSampleJSON(md *desc.MessageDescriptor) (string, error) {
	if md == nil {
		return "{}", nil
	}
	sampleMap := generateSampleValueForMessage(md, make(map[string]bool))
	bytes, err := json.MarshalIndent(sampleMap, "", "  ")
	if err != nil {
		return "{}", fmt.Errorf("failed to marshal sample message: %w", err)
	}
	return string(bytes), nil
}

func generateSampleValueForMessage(md *desc.MessageDescriptor, visited map[string]bool) map[string]interface{} {
	result := make(map[string]interface{})
	msgName := md.GetFullyQualifiedName()
	if visited[msgName] {
		return result // prevent infinite recursion on self-referential schemas
	}
	visited[msgName] = true
	defer delete(visited, msgName)

	for _, fd := range md.GetFields() {
		fieldName := fd.GetJSONName()
		if fieldName == "" {
			fieldName = fd.GetName()
		}

		val := generateSampleValueForField(fd, visited)
		if fd.IsRepeated() && !fd.IsMap() {
			result[fieldName] = []interface{}{val}
		} else if fd.IsMap() {
			result[fieldName] = map[string]interface{}{"key": "value"}
		} else {
			result[fieldName] = val
		}
	}
	return result
}

func generateSampleValueForField(fd *desc.FieldDescriptor, visited map[string]bool) interface{} {
	switch fd.GetType() {
	case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
		return true
	case descriptorpb.FieldDescriptorProto_TYPE_INT32,
		descriptorpb.FieldDescriptorProto_TYPE_SINT32,
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED32:
		return 0
	case descriptorpb.FieldDescriptorProto_TYPE_INT64,
		descriptorpb.FieldDescriptorProto_TYPE_SINT64,
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED64:
		return 0
	case descriptorpb.FieldDescriptorProto_TYPE_UINT32,
		descriptorpb.FieldDescriptorProto_TYPE_FIXED32:
		return 0
	case descriptorpb.FieldDescriptorProto_TYPE_UINT64,
		descriptorpb.FieldDescriptorProto_TYPE_FIXED64:
		return 0
	case descriptorpb.FieldDescriptorProto_TYPE_FLOAT,
		descriptorpb.FieldDescriptorProto_TYPE_DOUBLE:
		return 0.0
	case descriptorpb.FieldDescriptorProto_TYPE_STRING:
		return fmt.Sprintf("sample_%s", fd.GetName())
	case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		return "c2FtcGxl" // base64 "sample"
	case descriptorpb.FieldDescriptorProto_TYPE_ENUM:
		ed := fd.GetEnumType()
		if ed != nil && len(ed.GetValues()) > 0 {
			return ed.GetValues()[0].GetName()
		}
		return 0
	case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE:
		subMsg := fd.GetMessageType()
		if subMsg != nil {
			// Special handling for standard google.protobuf types
			switch subMsg.GetFullyQualifiedName() {
			case "google.protobuf.Timestamp":
				return "2026-10-03T12:00:00Z"
			case "google.protobuf.Duration":
				return "1s"
			case "google.protobuf.StringValue":
				return "sample"
			case "google.protobuf.Int32Value", "google.protobuf.Int64Value":
				return 0
			case "google.protobuf.BoolValue":
				return true
			case "google.protobuf.Struct":
				return map[string]interface{}{"key": "value"}
			default:
				return generateSampleValueForMessage(subMsg, visited)
			}
		}
		return map[string]interface{}{}
	default:
		return nil
	}
}
