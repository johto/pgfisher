package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	bolt "go.etcd.io/bbolt"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s DB_PATH\n", os.Args[0])
		os.Exit(1)
	}
	dbPath := os.Args[1]

	dbh, err := bolt.Open(dbPath, 0644, &bolt.Options{Timeout: time.Second})
	if err != nil {
		panic(err)
	}

	var dumpBucket func(bucket *bolt.Bucket) (map[string]interface{}, error)
	dumpBucket = func(bucket *bolt.Bucket) (map[string]interface{}, error) {
		data := make(map[string]interface{})
		bucket.ForEach(func(key []byte, value []byte) error {
			if value == nil {
				nestedBucket := bucket.Bucket(key)
				if nestedBucket == nil {
					panic(key)
				}
				dumpedData, err := dumpBucket(nestedBucket)
				if err != nil {
					return err
				}
				data[string(key)] = dumpedData
			} else {
				var dumped interface{}
				err := json.Unmarshal(bucket.Get(key), &dumped)
				if err != nil {
					panic(err)
				}
				data[string(key)] = dumped
			}
			return nil
		})
		return data, nil
	}
	datas := make(map[string]interface{})
	err = dbh.View(func(tx *bolt.Tx) error {
		return tx.ForEach(func(name []byte, b *bolt.Bucket) error {
			dumped, err := dumpBucket(b)
			if err != nil {
				return err
			}
			datas[string(name)] = dumped
			return nil
		})
	})
	if err != nil {
		panic(err)
	}
	jsonDatas, err := json.MarshalIndent(datas, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(jsonDatas))
}
