package main

import (
	"encoding/json"
	"fmt"
	"log"

	shared "github.com/johto/pgfisher/internal/plugin_interface"
	bolt "go.etcd.io/bbolt"
)

type PGFisherDatabase struct {
	dbh *bolt.DB
}

func NewPGFisherDatabase(dbh *bolt.DB) *PGFisherDatabase {
	return &PGFisherDatabase{
		dbh: dbh,
	}
}

func (db *PGFisherDatabase) InitializeDatabase(pos *shared.LogStreamPosition) {
	err := db.dbh.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucket([]byte("pgfisher"))
		if err != nil {
			return err
		}
		err = bucket.Put([]byte("logStreamPosition"), []byte("<uninitialized>"))
		if err != nil {
			return err
		}
		return err
	})
	if err != nil {
		log.Fatalf("could not update database: %s", err)
	}
	db.PersistLogStreamPosition(pos)
}

func (db *PGFisherDatabase) PersistLogStreamPosition(pos *shared.LogStreamPosition) {
	data, err := json.Marshal(pos)
	if err != nil {
		panic(err)
	}

	err = db.dbh.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("pgfisher"))
		if bucket == nil {
			panic("nil bucket")
		}
		return bucket.Put([]byte("logStreamPosition"), data)
	})
	if err != nil {
		log.Fatalf("could not write to %s: %s", db.dbh.Path(), err)
	}
}

func (db *PGFisherDatabase) ReadLogStreamPosition() shared.LogStreamPosition {
	var streamPosition shared.LogStreamPosition
	err := db.dbh.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("pgfisher"))
		if bucket == nil {
			panic("nil bucket")
		}
		data := bucket.Get([]byte("logStreamPosition"))
		if data == nil {
			panic("nil logStreamPosition")
		}
		err := json.Unmarshal(data, &streamPosition)
		if err != nil {
			panic(fmt.Errorf("could not unmarshal logStreamPosition: %s", err))
		}
		return nil
	})
	if err != nil {
		panic(fmt.Errorf("could not fetch initial position from %s: %s", db.dbh.Path(), err))
	}
	return streamPosition
}

func (db *PGFisherDatabase) BoltDBHandle() *bolt.DB {
	return db.dbh
}
