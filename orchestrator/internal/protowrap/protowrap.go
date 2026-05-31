package protowrap

import (
	"reflect"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func SetMessage(container proto.Message, value proto.Message) bool {
	if container == nil || value == nil || isNilPointer(value) {
		return false
	}
	message := value.ProtoReflect()
	field := messageField(container, message.Descriptor().FullName())
	if field == nil {
		return false
	}
	container.ProtoReflect().Set(field, protoreflect.ValueOfMessage(message))
	return true
}

func FirstMessage(container proto.Message) proto.Message {
	if container == nil || isNilPointer(container) {
		return nil
	}
	message := container.ProtoReflect()
	fields := message.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		if !message.Has(field) || field.Kind() != protoreflect.MessageKind {
			continue
		}
		value := message.Get(field).Message()
		if value.IsValid() {
			return value.Interface()
		}
	}
	return nil
}

func SetStringField(container proto.Message, fieldName string, value string) bool {
	if container == nil || isNilPointer(container) {
		return false
	}
	field := container.ProtoReflect().Descriptor().Fields().ByName(protoreflect.Name(fieldName))
	if field == nil || field.Kind() != protoreflect.StringKind {
		return false
	}
	container.ProtoReflect().Set(field, protoreflect.ValueOfString(value))
	return true
}

func messageField(container proto.Message, name protoreflect.FullName) protoreflect.FieldDescriptor {
	fields := container.ProtoReflect().Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		if field.Kind() == protoreflect.MessageKind && field.Message().FullName() == name {
			return field
		}
	}
	return nil
}

func isNilPointer(value proto.Message) bool {
	raw := reflect.ValueOf(value)
	return raw.Kind() == reflect.Pointer && raw.IsNil()
}
