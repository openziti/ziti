grammar ZitiQl;

// Allow case insensitive token definitions
fragment A : [aA];
fragment B : [bB];
fragment C : [cC];
fragment D : [dD];
fragment E : [eE];
fragment F : [fF];
fragment G : [gG];
fragment H : [hH];
fragment I : [iI];
fragment J : [jJ];
fragment K : [kK];
fragment L : [lL];
fragment M : [mM];
fragment N : [nN];
fragment O : [oO];
fragment P : [pP];
fragment Q : [qQ];
fragment R : [rR];
fragment S : [sS];
fragment T : [tT];
fragment U : [uU];
fragment V : [vV];
fragment W : [wW];
fragment X : [xX];
fragment Y : [yY];
fragment Z : [zZ];

WS: [ \n\t\r];

LPAREN: '(';
RPAREN: ')';
LBRACKET: '[';
RBRACKET: ']';

AND: A N D;
OR: O R;

LT: '<' '='?;
GT: '>' '='?;
EQ: '!'? '=';
CONTAINS: (N O T WS+)? C O N T A I N S;


IN: (N O T WS)? I N;
BETWEEN: (N O T WS+)? B E T W E E N;

BOOL: TRUE | FALSE;

fragment TRUE: T R U E;
fragment  FALSE: F A L S E;

DATETIME: 'datetime(' WS* RFC3339_DATE_TIME WS* ')';

fragment FULL_DATE: YEAR '-' MONTH '-' DAY;
fragment FULL_TIME: HOUR  ':' MINUTE ':' SECOND ('.' SECOND_FRACTION)?  TIME_NUM_OFFSET;
fragment YEAR: INT+;
fragment MONTH: ('0'[1-9]|'1'[012]);
fragment DAY: ('0'[1-9]|[12][0-9]|'3'[01]);
fragment HOUR: ([01][0-9]|'2'[0-3]);
fragment MINUTE: ([0-5][0-9]);
fragment SECOND: ([0-5][0-9]|'60');
fragment SECOND_FRACTION: [0-9]+;
fragment TIME_NUM_OFFSET: (Z|(('+'|'-') TIME_NUM_OFFSET_HOUR ':' TIME_NUM_OFFSET_MINUTE));
fragment TIME_NUM_OFFSET_HOUR: ([01][0-9]|'2'[0-3]);
fragment TIME_NUM_OFFSET_MINUTE: ([0-5][0-9]);

ALL_OF: A L L O F;
ANY_OF: A N Y O F;
COUNT: C O U N T;
ISEMPTY: I S E M P T Y;

STRING: '"' (ESC | SAFECODEPOINT)* '"';
NUMBER: '-'? INT ('.' [0-9] +)? EXP?;
NULL: N U L L;
NOT : N O T;

ASC: A S C;
DESC: D E S C;

SORT: S O R T;
BY: B Y;
SKIP_ROWS: S K I P;
LIMIT_ROWS: L I M I T;
NONE: N O N E;
WHERE: W H E R E;
FROM: F R O M;

fragment INT: '0' | [1-9] [0-9]*;
fragment EXP: [Ee] [+\-]? INT;

//Allow escaping of form-feed, new line, line feed, and tab. No support for backspace and other escapables
fragment ESC: '\\' ["\\fnrt];
fragment IDENTSEP: '.';
IDENTIFIER:
      [A-Za-z] [A-Za-z_]* (IDENTSEP [A-Za-z] [A-Za-z_]*)*
    | '\'' [A-Za-z] [A-Za-z_]* (IDENTSEP [A-Za-z] [A-Za-z_]*)* '\'';

// Per RFC
RFC3339_DATE_TIME: FULL_DATE  T  FULL_TIME;

//No control characters
fragment SAFECODEPOINT: ~ ["\\\u0000-\u001F];

// Empty lists not supported as some RDBMs don't allow empty lists
stringArray: LBRACKET WS* STRING (WS* ',' WS* STRING)* WS* RBRACKET;
numberArray: LBRACKET WS* NUMBER (WS* ',' WS* NUMBER)* WS* RBRACKET;
datetimeArray: LBRACKET WS* DATETIME (WS* ',' WS* DATETIME)* WS* RBRACKET;

start: WS* query WS* EOF #End;

query:
    boolExpr (WS+ sortBy)? (WS+ skip)? (WS+ limit)? #QueryStmt
    | sortBy (WS+ skip)? (WS+ limit)? #QueryStmt
    | skip (WS+ limit)? #QueryStmt
    | limit #QueryStmt;


skip: SKIP_ROWS WS+ NUMBER #SkipExpr;

limit: LIMIT_ROWS WS+ (NONE|NUMBER) #LimitExpr;

sortBy: SORT WS+ BY WS+ sortField (WS* ',' WS* sortField)* #SortByExpr;

sortField: IDENTIFIER (WS+ (ASC | DESC))? #SortFieldExpr;

boolExpr:
  operation #OperationOp
  | LPAREN WS* boolExpr WS* RPAREN #Group
  | boolExpr (WS+ AND WS+ boolExpr)+ #AndExpr
  | boolExpr (WS+ OR WS+ boolExpr)+ #OrExpr
  | BOOL #BoolConst
  | ISEMPTY LPAREN WS* setExpr WS* RPAREN #IsEmptyFunction
  | IDENTIFIER #BoolSymbol
  | NOT WS+ boolExpr #NotExpr
  ;

operation:
    binaryLhs WS+ IN WS+ stringArray #InStringArrayOp
  | binaryLhs WS+ IN WS+ numberArray #InNumberArrayOp
  | binaryLhs WS+ IN WS+ datetimeArray #InDatetimeArrayOp
  | binaryLhs WS+ BETWEEN WS+ NUMBER WS+ AND WS+ NUMBER # BetweenNumberOp
  | binaryLhs WS+ BETWEEN WS+ DATETIME WS+ AND WS+ DATETIME # BetweenDateOp
  | binaryLhs WS* LT WS* STRING # BinaryLessThanStringOp
  | binaryLhs WS* LT WS* NUMBER # BinaryLessThanNumberOp
  | binaryLhs WS* LT WS* DATETIME # BinaryLessThanDatetimeOp
  | binaryLhs WS* GT WS* STRING # BinaryGreaterThanStringOp
  | binaryLhs WS* GT WS* NUMBER # BinaryGreaterThanNumberOp
  | binaryLhs WS* GT WS* DATETIME# BinaryGreaterThanDatetimeOp
  | binaryLhs WS* EQ WS* STRING #BinaryEqualToStringOp
  | binaryLhs WS* EQ WS* NUMBER #BinaryEqualToNumberOp
  | binaryLhs WS* EQ WS* DATETIME #BinaryEqualToDatetimeOp
  | binaryLhs WS* EQ WS* BOOL #BinaryEqualToBoolOp
  | binaryLhs WS* EQ WS* NULL #BinaryEqualToNullOp
  | binaryLhs WS* CONTAINS WS+ (STRING|NUMBER) #BinaryContainsOp
  ;

binaryLhs: IDENTIFIER | setFunction;

setFunction:
    ALL_OF LPAREN WS* IDENTIFIER WS* RPAREN #SetFunctionExpr
  | ANY_OF LPAREN WS* IDENTIFIER WS* RPAREN #SetFunctionExpr
  | COUNT LPAREN WS* setExpr WS* RPAREN #SetFunctionExpr
  ;

setExpr:
    IDENTIFIER
  | subQueryExpr
  ;

subQueryExpr: FROM WS+ IDENTIFIER WS+ WHERE WS+ query #SubQuery;