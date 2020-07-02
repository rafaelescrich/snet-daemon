package escrow

import (
	log "github.com/sirupsen/logrus"
	"reflect"
	"strings"
)

// AtomicStorage is an interface to key-value storage with atomic operations.
type AtomicStorage interface {
	// Get returns value by key. ok value indicates whether passed key is
	// present in the storage. err indicates storage error.
	Get(key string) (value string, ok bool, err error)
	// GetByKeyPrefix returns list of values which keys has given prefix.
	GetByKeyPrefix(prefix string) (values []string, err error)
	// Put uncoditionally writes value by key in storage, err is not nil in
	// case of storage error.
	Put(key string, value string) (err error)
	// PutIfAbsent writes value if and only if key is absent in storage. ok is
	// true if key was absent and false otherwise. err indicates storage error.
	PutIfAbsent(key string, value string) (ok bool, err error)
	// CompareAndSwap atomically replaces prevValue by newValue. If ok flag is
	// true and err is nil then operation was successful. If err is nil and ok
	// is false then operation failed because prevValue is not equal to current
	// value. err indicates storage error.
	CompareAndSwap(key string, prevValue string, newValue string) (ok bool, err error)
	// Delete removes value by key
	Delete(key string) (err error)

	StartTransaction(conditionKeys []string) (transaction Transaction, err error)
	CompleteTransaction(transaction Transaction, update []*KeyValueData) (ok bool, err error)
	ExecuteTransaction(request CASRequest) (ok bool, err error)
}

type Transaction interface {
	GetConditionValues() ([]*KeyValueData, error)
}

//Best to change this to KeyValueData , will do this in the next commit
type UpdateFunc func(conditionValues []*KeyValueData) (update []*KeyValueData, err error)

type CASRequest struct {
	RetryTillSuccessOrError bool
	Update                  UpdateFunc
	ConditionKeys           []string
}

type KeyValueData struct {
	Key     string
	Value   string
	Present bool
}

// PrefixedAtomicStorage is decorator for atomic storage which adds a prefix to
// the storage keys.
type PrefixedAtomicStorage struct {
	delegate  AtomicStorage
	keyPrefix string
}

//It is recommended to use this function to create a PrefixedAtomicStorage
func NewPrefixedAtomicStorage(atomicStorage AtomicStorage, prefix string) *PrefixedAtomicStorage {
	return &PrefixedAtomicStorage{
		delegate:  atomicStorage,
		keyPrefix: prefix,
	}
}

// Get is implementation of AtomicStorage.Get
func (storage *PrefixedAtomicStorage) Get(key string) (value string, ok bool, err error) {
	return storage.delegate.Get(storage.keyPrefix + "/" + key)
}

func (storage *PrefixedAtomicStorage) GetByKeyPrefix(prefix string) (values []string, err error) {
	return storage.delegate.GetByKeyPrefix(storage.keyPrefix + "/" + prefix)
}

// Put is implementation of AtomicStorage.Put
func (storage *PrefixedAtomicStorage) Put(key string, value string) (err error) {
	return storage.delegate.Put(storage.keyPrefix+"/"+key, value)
}

// PutIfAbsent is implementation of AtomicStorage.PutIfAbsent
func (storage *PrefixedAtomicStorage) PutIfAbsent(key string, value string) (ok bool, err error) {
	return storage.delegate.PutIfAbsent(storage.keyPrefix+"/"+key, value)
}

// CompareAndSwap is implementation of AtomicStorage.CompareAndSwap
func (storage *PrefixedAtomicStorage) CompareAndSwap(key string, prevValue string, newValue string) (ok bool, err error) {
	return storage.delegate.CompareAndSwap(storage.keyPrefix+"/"+key, prevValue, newValue)
}

func (storage *PrefixedAtomicStorage) Delete(key string) (err error) {
	return storage.delegate.Delete(storage.keyPrefix + "/" + key)
}

func (storage *PrefixedAtomicStorage) StartTransaction(conditionKeys []string) (transaction Transaction, err error) {
	return storage.delegate.StartTransaction(conditionKeys)
}

func (storage *PrefixedAtomicStorage) appendKeyPrefix(conditionKeys []string) (preFixedConditionKeys []string) {
	preFixedConditionKeys = make([]string, len(conditionKeys))
	for i, key := range conditionKeys {
		preFixedConditionKeys[i] = storage.keyPrefix + "/" + key
	}
	return preFixedConditionKeys
}

func (storage *PrefixedAtomicStorage) appendKeyPrefixOn(update []*KeyValueData) []*KeyValueData {
	for _, keyValue := range update {
		keyValue.Key = storage.keyPrefix + "/" + keyValue.Key
	}
	return update
}

func (storage *PrefixedAtomicStorage) appendKeyPrefixOnCASRequest(request CASRequest) []string {
	prefixedKeys := make([]string, len(request.ConditionKeys))
	for i, key := range request.ConditionKeys {
		prefixedKeys[i] = storage.keyPrefix + "/" + key
	}
	return prefixedKeys
}

func (storage *PrefixedAtomicStorage) removeKeyPrefixOn(update []*KeyValueData) {
	for _, keyValue := range update {
		originalKey := keyValue.Key
		prefixRemovedkey := strings.Replace(originalKey, storage.keyPrefix+"/", "", -1)
		keyValue.Key = prefixRemovedkey
	}
}

func (storage *PrefixedAtomicStorage) CompleteTransaction(transaction Transaction, update []*KeyValueData) (ok bool, err error) {
	return storage.delegate.CompleteTransaction(transaction, storage.appendKeyPrefixOn(update))
}

func (storage *PrefixedAtomicStorage) ExecuteTransaction(request CASRequest) (ok bool, err error) {
	updateFunction := func(conditionValues []*KeyValueData) (update []*KeyValueData, err error) {
		//the keys retrieved will have the storage prefix, we need to remove it ! else deserizalize of key will fail
		storage.removeKeyPrefixOn(conditionValues)
		newValues, err := request.Update(conditionValues)
		storage.appendKeyPrefixOn(newValues)
		return newValues, err
	}
	prefixedRequest := CASRequest{ConditionKeys: storage.appendKeyPrefixOnCASRequest(request),
		RetryTillSuccessOrError: request.RetryTillSuccessOrError, Update: updateFunction}
	return storage.delegate.ExecuteTransaction(prefixedRequest)
}

// TypedAtomicStorage is an atomic storage which automatically
// serializes/deserializes values and keys
type TypedAtomicStorage interface {
	// Get returns value by key
	Get(key interface{}) (value interface{}, ok bool, err error)
	// GetAll returns an array which contains all values from storage
	GetAll() (array interface{}, err error)
	// Put puts value by key unconditionally
	Put(key interface{}, value interface{}) (err error)
	// PutIfAbsent puts value by key if and only if key is absent in storage
	PutIfAbsent(key interface{}, value interface{}) (ok bool, err error)
	// CompareAndSwap puts newValue by key if and only if previous value is equal
	// to prevValue
	CompareAndSwap(key interface{}, prevValue interface{}, newValue interface{}) (ok bool, err error)
	// Delete removes value by key
	Delete(key interface{}) (err error)
	ExecuteTransaction(request TypedCASRequest) (ok bool, err error)
}

type TypedTransaction interface {
	GetConditionValues() ([]*TypedKeyValueData, error)
}

//Best to change this to KeyValueData , will do this in the next commit
type TypedUpdateFunc func(conditionValues []*TypedKeyValueData) (update []*TypedKeyValueData, err error)

type TypedCASRequest struct {
	RetryTillSuccessOrError bool
	Update                  TypedUpdateFunc
	ConditionKeys           []interface{} //Typed Keys
}

type TypedKeyValueData struct {
	Key     interface{}
	Value   interface{}
	Present bool
}

// TypedAtomicStorageImpl is an implementation of TypedAtomicStorage interface
type TypedAtomicStorageImpl struct {
	atomicStorage     AtomicStorage
	keySerializer     func(key interface{}) (serialized string, err error)
	keyDeserializer   func(serialized string, key interface{}) (err error)
	keyType           reflect.Type
	valueSerializer   func(value interface{}) (serialized string, err error)
	valueDeserializer func(serialized string, value interface{}) (err error)
	valueType         reflect.Type
}

// Get implements TypedAtomicStorage.Get
func (storage *TypedAtomicStorageImpl) Get(key interface{}) (value interface{}, ok bool, err error) {
	keyString, err := storage.keySerializer(key)
	if err != nil {
		return
	}

	valueString, ok, err := storage.atomicStorage.Get(keyString)
	if err != nil {
		return
	}
	if !ok {
		return
	}

	value, err = storage.deserializeValue(valueString)
	if err != nil {
		return nil, false, err
	}

	return value, true, nil
}

func (storage *TypedAtomicStorageImpl) deserializeKey(keyString string) (key interface{}, err error) {
	key = reflect.New(storage.keyType).Interface()
	err = storage.keyDeserializer(keyString, key)
	if err != nil {
		return nil, err
	}
	return key, err
}

func (storage *TypedAtomicStorageImpl) deserializeValue(valueString string) (value interface{}, err error) {
	value = reflect.New(storage.valueType).Interface()
	err = storage.valueDeserializer(valueString, value)
	if err != nil {
		return nil, err
	}
	return value, err
}

func (storage *TypedAtomicStorageImpl) GetAll() (array interface{}, err error) {
	stringValues, err := storage.atomicStorage.GetByKeyPrefix("")
	if err != nil {
		return
	}

	values := reflect.MakeSlice(
		reflect.SliceOf(reflect.PtrTo(storage.valueType)),
		0, len(stringValues))

	for _, stringValue := range stringValues {
		value := reflect.New(storage.valueType)
		err = storage.valueDeserializer(stringValue, value.Interface())
		if err != nil {
			return nil, err
		}
		values = reflect.Append(values, value)
	}

	return values.Interface(), nil
}

// Put implementor TypedAtomicStorage.Put
func (storage *TypedAtomicStorageImpl) Put(key interface{}, value interface{}) (err error) {
	keyString, err := storage.keySerializer(key)
	if err != nil {
		return
	}

	valueString, err := storage.valueSerializer(value)
	if err != nil {
		return
	}

	return storage.atomicStorage.Put(keyString, valueString)
}

// PutIfAbsent implements TypedAtomicStorage.PutIfAbsent
func (storage *TypedAtomicStorageImpl) PutIfAbsent(key interface{}, value interface{}) (ok bool, err error) {
	keyString, err := storage.keySerializer(key)
	if err != nil {
		return
	}

	valueString, err := storage.valueSerializer(value)
	if err != nil {
		return
	}

	return storage.atomicStorage.PutIfAbsent(keyString, valueString)
}

// CompareAndSwap implements TypedAtomicStorage.CompareAndSwap
func (storage *TypedAtomicStorageImpl) CompareAndSwap(key interface{}, prevValue interface{}, newValue interface{}) (ok bool, err error) {
	keyString, err := storage.keySerializer(key)
	if err != nil {
		return
	}

	newValueString, err := storage.valueSerializer(newValue)
	if err != nil {
		return
	}

	prevValueString, err := storage.valueSerializer(prevValue)
	if err != nil {
		return
	}

	return storage.atomicStorage.CompareAndSwap(keyString, prevValueString, newValueString)
}

func (storage *TypedAtomicStorageImpl) Delete(key interface{}) (err error) {
	keyString, err := storage.keySerializer(key)
	if err != nil {
		return
	}

	return storage.atomicStorage.Delete(keyString)
}

type typedTransactionImpl struct {
	transactionString Transaction
	storage           *TypedAtomicStorageImpl
}

func (transaction *typedTransactionImpl) GetConditionValues() ([]*TypedKeyValueData, error) {
	keyValueDataString, err := transaction.transactionString.GetConditionValues()
	if err != nil {
		return nil, err
	}
	return transaction.storage.convertKeyValueDataToTyped(keyValueDataString)
}

func (storage *TypedAtomicStorageImpl) convertKeyValueDataToTyped(keyValueData []*KeyValueData) ([]*TypedKeyValueData, error) {
	result := make([]*TypedKeyValueData, len(keyValueData))
	for i, keyValueString := range keyValueData {
		key, err := storage.deserializeKey(keyValueString.Key)
		if err != nil {
			return nil, err
		}
		value, err := storage.deserializeValue(keyValueString.Value)
		if err != nil {
			return nil, err
		}
		result[i] = &TypedKeyValueData{
			Key:     key,
			Value:   value,
			Present: keyValueString.Present,
		}
	}
	return result, nil
}

func (storage *TypedAtomicStorageImpl) ExecuteTransaction(request TypedCASRequest) (ok bool, err error) {

	updateFunction := func(conditionValues []*KeyValueData) (update []*KeyValueData, err error) {
		typedValues, err := storage.convertKeyValueDataToTyped(conditionValues)
		if err != nil {
			return nil, err
		}
		typedUpdate, err := request.Update(typedValues)
		if err != nil {
			return nil, err
		}
		return storage.convertTypedKeyValueDataToString(typedUpdate)
	}

	storageRequest := CASRequest{
		RetryTillSuccessOrError: request.RetryTillSuccessOrError,
		Update:                  updateFunction,
		ConditionKeys:           storage.convertTypedKeyToString(request.ConditionKeys),
	}
	return storage.atomicStorage.ExecuteTransaction(storageRequest)
}

func (storage *TypedAtomicStorageImpl) convertTypedKeyToString(typedKeys []interface{}) []string {
	stringKeys := make([]string, len(typedKeys))
	for i, key := range typedKeys {
		serializedKey, err := storage.keySerializer(key)
		stringKeys[i] = serializedKey
		if err != nil {
			log.Error(err)
		}
	}
	return stringKeys
}
func (storage *TypedAtomicStorageImpl) StartTransaction(conditionKeys []string) (transaction TypedTransaction, err error) {
	transactionString, err := storage.atomicStorage.StartTransaction(conditionKeys)
	if err != nil {
		return
	}

	return &typedTransactionImpl{
		transactionString: transactionString,
		storage:           storage,
	}, nil
}

func (storage *TypedAtomicStorageImpl) convertTypedKeyValueDataToString(
	update []*TypedKeyValueData) (data []*KeyValueData, err error) {
	updateString := make([]*KeyValueData, len(update))
	for i, keyValue := range update {
		key, err := storage.keySerializer(keyValue.Key)
		if err != nil {
			return nil, err
		}
		value, err := storage.valueSerializer(keyValue.Value)
		if err != nil {
			return nil, err
		}
		updateString[i] = &KeyValueData{
			Key:   key,
			Value: value,
		}
	}
	return updateString, nil
}
func (storage *TypedAtomicStorageImpl) CompleteTransaction(transaction TypedTransaction, update []*TypedKeyValueData) (ok bool, err error) {
	updateString, err := storage.convertTypedKeyValueDataToString(update)
	if err != nil {
		return false, err
	}
	return storage.atomicStorage.CompleteTransaction(transaction.(*typedTransactionImpl).transactionString, updateString)
}
