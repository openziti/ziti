The query package contains the following:

1. An AST (Abstract Syntax Tree) for filter expressions
2. An Antlr listener/AST visitor which can transform ZitiQL into the filter AST

The general pattern is that when it hits leaf nodes it processes them into filter AST nodes and pushes the results onto a stack. Non-leaf nodes then pop the nodes they need off the stack, create their corresponding filter node and push that onto the stack, and so on up the tree.

Ex: foo = 2

The following get pushed onto the stack

* UntypedSymbolNode(foo)
* BinaryOpEq
* Int64ConstNode(2)

Then a BinaryExprNode is created by popping those three items off the stack.

Typing is done in a second pass, where UntypedSymbolNode is converted to a typed symbol node, hoping an Int64SymbolNode and the BinaryExprNode is converted to an Int64BinaryExprNode.

If you're curious as to what kind of elements make up the AST, take a look at visitor.go. There's a Visit<NodeType>(node *<NodeType>) for every leaf node type and Visit*Start/Visit*End for every non-leaf node type.

There are a few node types which are used as place holders until we can apply type information to the AST, such as

1. BinaryExprNode
2. InArrayExprNode
3. BetweenExprNode
4. UntypedSymbolNode
5. SetFunctionNode

These are replaced by more specific typed nodes during the typing pass.

Type information is expressed via a Symbols interface, which allows retrieving symbol types as well as evaluating symbols. 
The filter AST provides for the following types:

1. Bool
2. String
3. Int64
4. Float64
5. Datetime
6. Nil

The underlying datastore may convert other types to these types (for example int32 -> int64). Providing more types would provide more flexability and better performance at the cost of more complexity and larger permutations of supported conversions. Int64/Float64 should provide good performance and enough space. If we find we need either better perf by more using smaller types or support for bigint/bigfloat, we can add that later.

Supported automatic type conversions:

1. int64 can be converted to float64. 
2. int64 and float64 can be converted to string

Other type conversions are certainly possible and may be added later.

The Symbols type is the interface which is to be implemented by the underlying data store. The root of a filter AST should be a BoolNode, on which you can call `EvalBool(s Symbols) (bool, error)`. 

Set Functions
The grammer provides for a few set functions

* anyOf
* allOf

These can be used to wrap a symbol. Ex: when filtering services, `anyOf(appWans.identity) = <current-user-id>`. 
This gets converted to `anyOf(appWans.identity = <current-user-id>)`. The elements of the set appWans.identity are inspected and if at least one matches the condition, it will return true. 

This is an entity-centric kind of set algebra and is limited in a few ways. It's not a like a true join where you get a full set for each permutation of the source and joined data. Rather you just iterate a set related to the original entity. You also can't do multiple expressions within the set operation. However, it's good enough for the use cases we want to cover here. 
