package main

import (
	"errors"

	"github.com/dgraph-io/badger/v4"
)

type badgerKV struct {
	db *badger.DB
}

func openBadgerKV(name string) (kv, error) {
	path, err := getFilePath(name)
	if err != nil {
		return nil, err
	}
	db, err := badger.Open(badger.DefaultOptions(path).WithLoggingLevel(badger.ERROR))
	if err != nil {
		return nil, err //nolint:wrapcheck
	}
	return badgerKV{db: db}, nil
}

//nolint:wrapcheck
func (b badgerKV) Get(key []byte) ([]byte, error) {
	var v []byte
	err := wrap(b.db, true, func(tx *badger.Txn) error {
		item, err := tx.Get(key)
		if err != nil {
			return err
		}
		v, err = item.ValueCopy(nil)
		return err
	})
	if errors.Is(err, badger.ErrKeyNotFound) {
		return nil, errKeyNotFound
	}
	return v, err
}

func (b badgerKV) Set(key, value []byte) error {
	return wrap(b.db, false, func(tx *badger.Txn) error {
		return tx.Set(key, value)
	})
}

func (b badgerKV) Delete(key []byte) error {
	return wrap(b.db, false, func(tx *badger.Txn) error {
		return tx.Delete(key)
	})
}

//nolint:wrapcheck
func (b badgerKV) Scan(opts scanOptions, fn func(key, value []byte) error) error {
	if err := b.db.Sync(); err != nil {
		return err
	}
	return b.db.View(func(txn *badger.Txn) error {
		io := badger.DefaultIteratorOptions
		io.PrefetchSize = 10
		io.Reverse = opts.reverse
		if opts.keysOnly {
			io.PrefetchValues = false
		}
		it := txn.NewIterator(io)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			if opts.keysOnly {
				if err := fn(item.Key(), nil); err != nil {
					return err
				}
				continue
			}
			if err := item.Value(func(v []byte) error {
				return fn(item.Key(), v)
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

//nolint:wrapcheck
func (b badgerKV) Close() error {
	return b.db.Close()
}

func wrap(db *badger.DB, readonly bool, fn func(tx *badger.Txn) error) error {
	tx := db.NewTransaction(!readonly)
	if err := fn(tx); err != nil {
		tx.Discard()
		return err
	}
	return tx.Commit() //nolint:wrapcheck
}
