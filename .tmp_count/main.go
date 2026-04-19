package main
import (
  "context"
  "fmt"
  "github.com/abartrim/sobs/internal/store"
)
func main(){
  f:=store.NewChdbStoreFactoryFromEnv()
  s, err := f.Open(context.Background())
  if err != nil { panic(err) }
  defer s.Close()
  rows, err := s.Query(context.Background(), "SELECT count() FROM sobs_ingest_opaque WHERE Path='/v1/rum'")
  if err != nil { panic(err) }
  defer rows.Close()
  var c uint64
  if rows.Next() {
    if err := rows.Scan(&c); err != nil { panic(err) }
  }
  fmt.Printf("v1_rum_rows=%d\n", c)
}
