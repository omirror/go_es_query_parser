### The Query DSL

The DSL is comprised of operators (`AND`, `OR`, and `NOT`), grouping symbols (parentheses), and value elements.
Values are comprised of either a single value, applied to the search query against a default field, or a key-value
pair, separated by a `:`. In the latter case, the keys are a single token representing a field in the indexed documents
you wish to search. Values are comprised of one of several possible data types, possibly including sub-operators corresponding
to query type to be applied.


Some value element examples:

`foo` ~ search the default field for value "foo" in a match or term query

`35` ~ search the default field for the number 35, as an integer in a match or term query

`name:Joe` ~ search the `name` field for the value "Joe" as a match or term query

`count:2` ~ search the `count` field for the numerical value 2 as a match or term query

`graduated:?` ~ search for documents where the `graduated` field exists

`msg:"foo bar baz"` ~ search the `msg` field using a match-phrase query

`amount:>=40` ~ search the `amount` field using a range query for documents where the field's value is greater than or equal to 40

`created_at:<2017-10-31T00:00:00Z` ~ search the `created_at` field for dates before Halloween of 2017 (_all datetimes are in RF3339 format, UTC timezone_)

`cash:[50~200]` ~ returns all docs where `cash` field's value is within a range greater than or equal to 50, and less than 200.

`updated_at:[2017-04-22T09:45:00Z~2017-05-03T10:20:00Z]` ~ window ranges can also include RFC3339 UTC datetimes


Any field or parenthesized grouping can be negated with the `NOT` or `!` operator:

`NOT foo` ~ search for documents where default field doesn't contain the token `foo`

`!c:[2017-10-29T00:00:00Z~2017-10-30T00:00:00Z]` ~ returns docs where field `c`'s date value is _not_ within the range of October 29-31, 2017 (UTC)

`NOT available:?` ~ search for documents where `available` field does not exist 

`!count:>100` ~ search for documents where `count` field has a value that's _not_ greater than 100

`NOT (x OR y)` ~ search the default field for documents that don't contain terms "x" or "y"


Parentheses are used for grouping of subqueries:

`a OR (b:"some words" AND NOT c:20)` ~ return docs containing term "a" or where field `b` matches the phrase "some words", but field `c`'s value is not 20.

`NOT foo:bar AND baz:99` ~ return docs where field `foo`'s value is not "bar" and where field `baz`'s value is 99.


Operators have aliases: `AND` -> `&&` and `OR` -> `||`:

`!(b:? || c:?) && a:1` ~ returns docs where neither fields `b` or `c` exist, but field `a` exists and is equal to 1. 


Nesting depth is arbitrary, limits are configured on the ES side:

`(a OR b OR (c:5 AND d:10)) AND NOT ((x:foo OR x:bar) AND y:? AND updated:<=2017-11-29T04:15:00Z) AND NOT z:[20~40]`


#### Gotchas/TODOs
* `AND`/`OR` can't be mixed within a single query clause: `(x AND (!y OR a))` is valid, but `(x AND !y OR a)` is not
* `AND` is the default query operator in each query clause at each nesting depth, to change this use `--default-or`
* `AND`/`OR` opers override the default in the AST walk, so default-`AND` query `(!x AND y) OR a` generates a different output than `a OR (!x AND y)`
* Single values or KV pairs are rendered as Match queries by default, and Term queries in filter context

