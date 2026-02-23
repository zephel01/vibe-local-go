package tool

// SchemaBuilder helps build function schemas
type SchemaBuilder struct {
	schema *FunctionSchema
}

// NewSchemaBuilder creates a new schema builder
func NewSchemaBuilder(name string) *SchemaBuilder {
	return &SchemaBuilder{
		schema: &FunctionSchema{
			Name:       name,
			Parameters: &ParameterSchema{
				Type:       "object",
				Properties: make(map[string]*PropertyDef),
				Required:   []string{},
			},
		},
	}
}

// SetDescription sets the function description
func (b *SchemaBuilder) SetDescription(desc string) *SchemaBuilder {
	b.schema.Description = desc
	return b
}

// AddString adds a string parameter
func (b *SchemaBuilder) AddString(name, desc string, required bool) *SchemaBuilder {
	b.schema.Parameters.Properties[name] = &PropertyDef{
		Type:        "string",
		Description: desc,
	}
	if required {
		b.schema.Parameters.Required = append(b.schema.Parameters.Required, name)
	}
	return b
}

// AddNumber adds a number parameter
func (b *SchemaBuilder) AddNumber(name, desc string, required bool) *SchemaBuilder {
	b.schema.Parameters.Properties[name] = &PropertyDef{
		Type:        "number",
		Description: desc,
	}
	if required {
		b.schema.Parameters.Required = append(b.schema.Parameters.Required, name)
	}
	return b
}

// AddInteger adds an integer parameter
func (b *SchemaBuilder) AddInteger(name, desc string, required bool) *SchemaBuilder {
	b.schema.Parameters.Properties[name] = &PropertyDef{
		Type:        "integer",
		Description: desc,
	}
	if required {
		b.schema.Parameters.Required = append(b.schema.Parameters.Required, name)
	}
	return b
}

// AddBoolean adds a boolean parameter
func (b *SchemaBuilder) AddBoolean(name, desc string, required bool) *SchemaBuilder {
	b.schema.Parameters.Properties[name] = &PropertyDef{
		Type:        "boolean",
		Description: desc,
	}
	if required {
		b.schema.Parameters.Required = append(b.schema.Parameters.Required, name)
	}
	return b
}

// AddArray adds an array parameter
func (b *SchemaBuilder) AddArray(name, desc string, itemType string, required bool) *SchemaBuilder {
	b.schema.Parameters.Properties[name] = &PropertyDef{
		Type:        "array",
		Description: desc,
		Items: &PropertyDef{
			Type: itemType,
		},
	}
	if required {
		b.schema.Parameters.Required = append(b.schema.Parameters.Required, name)
	}
	return b
}

// AddObject adds an object parameter
func (b *SchemaBuilder) AddObject(name, desc string, required bool) *SchemaBuilder {
	b.schema.Parameters.Properties[name] = &PropertyDef{
		Type:        "object",
		Description: desc,
		Properties:  make(map[string]*PropertyDef),
		Required:    []string{},
	}
	if required {
		b.schema.Parameters.Required = append(b.schema.Parameters.Required, name)
	}
	return b
}

// AddEnum adds an enum parameter
func (b *SchemaBuilder) AddEnum(name, desc string, values []string, required bool) *SchemaBuilder {
	b.schema.Parameters.Properties[name] = &PropertyDef{
		Type:        "string",
		Description: desc,
		Enum:        values,
	}
	if required {
		b.schema.Parameters.Required = append(b.schema.Parameters.Required, name)
	}
	return b
}

// Build builds and returns the function schema
func (b *SchemaBuilder) Build() *FunctionSchema {
	return b.schema
}

// PropertyDefBuilder helps build property definitions
type PropertyDefBuilder struct {
	prop *PropertyDef
}

// NewPropertyDefBuilder creates a new property definition builder
func NewPropertyDefBuilder(typ string) *PropertyDefBuilder {
	return &PropertyDefBuilder{
		prop: &PropertyDef{
			Type:       typ,
			Properties: make(map[string]*PropertyDef),
			Required:   []string{},
		},
	}
}

// SetDescription sets the property description
func (b *PropertyDefBuilder) SetDescription(desc string) *PropertyDefBuilder {
	b.prop.Description = desc
	return b
}

// SetDefault sets the default value
func (b *PropertyDefBuilder) SetDefault(value interface{}) *PropertyDefBuilder {
	b.prop.Default = value
	return b
}

// AddProperty adds a nested property
func (b *PropertyDefBuilder) AddProperty(name string, prop *PropertyDef) *PropertyDefBuilder {
	b.prop.Properties[name] = prop
	return b
}

// Build builds and returns the property definition
func (b *PropertyDefBuilder) Build() *PropertyDef {
	return b.prop
}
