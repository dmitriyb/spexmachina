# Schema Validation Implementation

## Approach

Use a Go JSON Schema validation library to validate parsed JSON against the embedded schemas. The `santhosh-tekuri/jsonschema` library is a strong candidate — pure Go, supports draft 2020-12, and returns detailed error paths.

## Algorithm

1. Load embedded schema bytes from the `schema` package
2. Compile the schema into a validator (done once at startup)
3. Read the target JSON file
4. Unmarshal into `interface{}` (not into typed structs — schema validation needs raw JSON)
5. Validate against the compiled schema
6. Convert each schema violation into a `ValidationError` with path and message

## Error Mapping

JSON Schema validation errors include a JSON pointer to the violating field and a description. Map these directly to `ValidationError.Path` and `ValidationError.Message`.

## Performance

Schema compilation is done once. Validation of each JSON file is a single pass. For 100 module.json files, this is 100 validation calls — well within the 1-second budget.
