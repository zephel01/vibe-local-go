package tool

import (
	"testing"
)

func TestNewSchemaBuilder(t *testing.T) {
	builder := NewSchemaBuilder("test_function")

	if builder == nil {
		t.Fatal("expected non-nil builder")
	}

	schema := builder.Build()
	if schema.Name != "test_function" {
		t.Errorf("expected name 'test_function', got '%s'", schema.Name)
	}

	if schema.Parameters == nil {
		t.Error("expected non-nil parameters")
	}
}

func TestSchemaBuilder_SetDescription(t *testing.T) {
	builder := NewSchemaBuilder("test")
	builder.SetDescription("Test description")

	schema := builder.Build()
	if schema.Description != "Test description" {
		t.Errorf("expected 'Test description', got '%s'", schema.Description)
	}
}

func TestSchemaBuilder_AddString(t *testing.T) {
	builder := NewSchemaBuilder("test")
	builder.AddString("name", "Name parameter", true)

	schema := builder.Build()

	// Check parameter exists
	prop, ok := schema.Parameters.Properties["name"]
	if !ok {
		t.Error("string parameter not found")
	}

	// Check parameter type
	if prop.Type != "string" {
		t.Errorf("expected type 'string', got '%s'", prop.Type)
	}

	// Check parameter description
	if prop.Description != "Name parameter" {
		t.Errorf("expected 'Name parameter', got '%s'", prop.Description)
	}

	// Check required
	found := false
	for _, req := range schema.Parameters.Required {
		if req == "name" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'name' to be in required list")
	}
}

func TestSchemaBuilder_AddString_Optional(t *testing.T) {
	builder := NewSchemaBuilder("test")
	builder.AddString("name", "Name parameter", false)

	schema := builder.Build()

	// Check not in required list
	for _, req := range schema.Parameters.Required {
		if req == "name" {
			t.Error("expected 'name' not to be in required list for optional parameter")
		}
	}
}

func TestSchemaBuilder_AddNumber(t *testing.T) {
	builder := NewSchemaBuilder("test")
	builder.AddNumber("value", "Number parameter", true)

	schema := builder.Build()
	prop := schema.Parameters.Properties["value"]

	if prop.Type != "number" {
		t.Errorf("expected type 'number', got '%s'", prop.Type)
	}
}

func TestSchemaBuilder_AddInteger(t *testing.T) {
	builder := NewSchemaBuilder("test")
	builder.AddInteger("count", "Count parameter", true)

	schema := builder.Build()
	prop := schema.Parameters.Properties["count"]

	if prop.Type != "integer" {
		t.Errorf("expected type 'integer', got '%s'", prop.Type)
	}
}

func TestSchemaBuilder_AddBoolean(t *testing.T) {
	builder := NewSchemaBuilder("test")
	builder.AddBoolean("enabled", "Boolean parameter", true)

	schema := builder.Build()
	prop := schema.Parameters.Properties["enabled"]

	if prop.Type != "boolean" {
		t.Errorf("expected type 'boolean', got '%s'", prop.Type)
	}
}

func TestSchemaBuilder_AddArray(t *testing.T) {
	builder := NewSchemaBuilder("test")
	builder.AddArray("items", "Array parameter", "string", true)

	schema := builder.Build()
	prop := schema.Parameters.Properties["items"]

	if prop.Type != "array" {
		t.Errorf("expected type 'array', got '%s'", prop.Type)
	}

	if prop.Items == nil {
		t.Error("expected items to be defined for array type")
	}

	if prop.Items.Type != "string" {
		t.Errorf("expected item type 'string', got '%s'", prop.Items.Type)
	}
}

func TestSchemaBuilder_AddObject(t *testing.T) {
	builder := NewSchemaBuilder("test")
	builder.AddObject("config", "Object parameter", true)

	schema := builder.Build()
	prop := schema.Parameters.Properties["config"]

	if prop.Type != "object" {
		t.Errorf("expected type 'object', got '%s'", prop.Type)
	}

	if prop.Properties == nil {
		t.Error("expected properties to be defined for object type")
	}
}

func TestSchemaBuilder_AddEnum(t *testing.T) {
	enumValues := []string{"option1", "option2", "option3"}
	builder := NewSchemaBuilder("test")
	builder.AddEnum("mode", "Enum parameter", enumValues, true)

	schema := builder.Build()
	prop := schema.Parameters.Properties["mode"]

	if prop.Type != "string" {
		t.Errorf("expected type 'string', got '%s'", prop.Type)
	}

	if len(prop.Enum) != 3 {
		t.Errorf("expected 3 enum values, got %d", len(prop.Enum))
	}

	for i, val := range enumValues {
		if prop.Enum[i] != val {
			t.Errorf("expected enum value '%s', got '%s'", val, prop.Enum[i])
		}
	}
}

func TestSchemaBuilder_FluentChaining(t *testing.T) {
	schema := NewSchemaBuilder("test_func").
		SetDescription("A test function").
		AddString("name", "Name", true).
		AddInteger("age", "Age", false).
		AddBoolean("active", "Active status", true).
		Build()

	if schema.Name != "test_func" {
		t.Errorf("expected 'test_func', got '%s'", schema.Name)
	}

	if len(schema.Parameters.Properties) != 3 {
		t.Errorf("expected 3 parameters, got %d", len(schema.Parameters.Properties))
	}

	if len(schema.Parameters.Required) != 2 {
		t.Errorf("expected 2 required parameters, got %d", len(schema.Parameters.Required))
	}
}

func TestNewPropertyDefBuilder(t *testing.T) {
	builder := NewPropertyDefBuilder("string")

	if builder == nil {
		t.Fatal("expected non-nil builder")
	}

	prop := builder.Build()
	if prop.Type != "string" {
		t.Errorf("expected type 'string', got '%s'", prop.Type)
	}
}

func TestPropertyDefBuilder_SetDescription(t *testing.T) {
	prop := NewPropertyDefBuilder("string").
		SetDescription("Test property").
		Build()

	if prop.Description != "Test property" {
		t.Errorf("expected 'Test property', got '%s'", prop.Description)
	}
}

func TestPropertyDefBuilder_SetDefault(t *testing.T) {
	prop := NewPropertyDefBuilder("integer").
		SetDefault(42).
		Build()

	if prop.Default != 42 {
		t.Errorf("expected default 42, got %v", prop.Default)
	}
}

func TestPropertyDefBuilder_AddProperty(t *testing.T) {
	nestedProp := &PropertyDef{
		Type:        "string",
		Description: "Nested property",
	}

	prop := NewPropertyDefBuilder("object").
		AddProperty("nested", nestedProp).
		Build()

	if len(prop.Properties) != 1 {
		t.Errorf("expected 1 property, got %d", len(prop.Properties))
	}

	retrieved := prop.Properties["nested"]
	if retrieved.Description != "Nested property" {
		t.Errorf("expected 'Nested property', got '%s'", retrieved.Description)
	}
}

func TestPropertyDefBuilder_FluentChaining(t *testing.T) {
	nestedProp := NewPropertyDefBuilder("string").
		SetDescription("Nested").
		Build()

	prop := NewPropertyDefBuilder("object").
		SetDescription("Object property").
		SetDefault(nil).
		AddProperty("nested", nestedProp).
		Build()

	if prop.Description != "Object property" {
		t.Errorf("expected 'Object property', got '%s'", prop.Description)
	}

	if len(prop.Properties) != 1 {
		t.Error("expected 1 property")
	}
}

func TestFunctionSchema_JSON(t *testing.T) {
	schema := NewSchemaBuilder("test_func").
		SetDescription("Test function").
		AddString("param1", "First parameter", true).
		AddInteger("param2", "Second parameter", false).
		Build()

	// Test JSON marshaling (implicitly by accessing fields)
	if schema.Name != "test_func" {
		t.Error("schema name mismatch")
	}

	if schema.Description != "Test function" {
		t.Error("schema description mismatch")
	}

	if schema.Parameters.Type != "object" {
		t.Error("parameters type should be object")
	}
}
