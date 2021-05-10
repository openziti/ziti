# Release 0.10

## Theme

Ziti 0.10 includes a single change:

* Edge API input validation processing was changed to operate on the supplied JSON instead of target
  objects

## Edge API Validation

Before this version, the Edge API allowed some fields to be omitted from requests. This behavior was
due to the fact that the API was validating the object that resulted from the JSON. This would cause
some fields that were not supplied to default to an acceptable nil/null/empty value.

Some APIs call may now fail with validation errors expecting fields to be defined for POST (create)
and PUT (update)
operations. PATCH (partial update) should not be affected.
