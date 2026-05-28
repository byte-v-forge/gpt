package resultdata

import (
	"fmt"

	"google.golang.org/protobuf/types/known/structpb"
)

type Builder struct {
	fields map[string]any
}

func New() Builder {
	return Builder{fields: map[string]any{}}
}

func From(fields map[string]any) Builder {
	builder := New()
	builder.Merge(fields)
	return builder
}

func (b Builder) Add(key string, value any) Builder {
	if key == "" || value == nil {
		return b
	}
	if b.fields == nil {
		b.fields = map[string]any{}
	}
	b.fields[key] = value
	return b
}

func (b Builder) AddStruct(key string, value *structpb.Struct) Builder {
	return b.Add(key, Map(value))
}

func (b Builder) Merge(fields map[string]any) Builder {
	if len(fields) == 0 {
		return b
	}
	if b.fields == nil {
		b.fields = map[string]any{}
	}
	for key, value := range fields {
		if key == "" || value == nil {
			continue
		}
		b.fields[key] = value
	}
	return b
}

func (b Builder) Map() map[string]any {
	if len(b.fields) == 0 {
		return nil
	}
	out := make(map[string]any, len(b.fields))
	for key, value := range b.fields {
		out[key] = value
	}
	return out
}

func (b Builder) Struct() *structpb.Struct {
	return Struct(b.Map())
}

func Struct(data map[string]any) *structpb.Struct {
	if len(data) == 0 {
		return nil
	}
	out, err := structpb.NewStruct(SanitizeMap(data))
	if err != nil {
		out, _ = structpb.NewStruct(map[string]any{"marshal_error": err.Error()})
	}
	return out
}

func SanitizeMap(data map[string]any) map[string]any {
	if len(data) == 0 {
		return nil
	}
	out := make(map[string]any, len(data))
	for key, value := range data {
		out[key] = sanitizeValue(value)
	}
	return out
}

func sanitizeValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case map[string]any:
		return SanitizeMap(typed)
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, sanitizeValue(item))
		}
		return out
	case []string:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	case []int:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	case []int64:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	case []float64:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	case []bool:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	case fmt.Stringer:
		return typed.String()
	default:
		return value
	}
}

func Map(data *structpb.Struct) map[string]any {
	if data == nil {
		return nil
	}
	return data.AsMap()
}
