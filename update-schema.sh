
get-graphql-schema http://hge.gamtrac.cndb.biocad.ru/v1/graphql > ./api/schema.graphql
go run scripts/gqlgen.go
rm generated.go
