// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package substrate

import (
	"errors"
	"fmt"
	"github.com/ChainSafe/log15"
	"github.com/centrifuge/chainbridge-utils/msg"
	"github.com/centrifuge/go-substrate-rpc-client/v4/registry"
	"github.com/centrifuge/go-substrate-rpc-client/v4/types"
)

type eventName string
type eventHandler func(registry.DecodedFields, log15.Logger) (msg.Message, error)

const FungibleTransfer eventName = "ChainBridge.FungibleTransfer"
const NonFungibleTransfer eventName = "ChainBridge.NonFungibleTransfer"
const GenericTransfer eventName = "ChainBridge.GenericTransfer"

var Subscriptions = []struct {
	name    eventName
	handler eventHandler
}{
	{FungibleTransfer, fungibleTransferHandler},
	{NonFungibleTransfer, nonFungibleTransferHandler},
	{GenericTransfer, genericTransferHandler},
}

func fungibleTransferHandler(eventFields registry.DecodedFields, log log15.Logger) (msg.Message, error) {
	chainID, err := getFieldValueAsType[types.U8]("ChainId", eventFields)
	if err != nil {
		return msg.Message{}, err
	}

	depositNonce, err := getFieldValueAsType[types.U64]("DepositNonce", eventFields)
	if err != nil {
		return msg.Message{}, err
	}

	resID, err := getFieldValueAsSliceOfType[types.U8]("ResourceId", eventFields)
	if err != nil {
		return msg.Message{}, err
	}

	resourceID, err := to32Bytes(resID)
	if err != nil {
		return msg.Message{}, err
	}

	amount, err := getFieldValueAsType[types.U256]("primitive_types.U256.U256", eventFields)
	if err != nil {
		return msg.Message{}, err
	}

	recipient, err := getFieldValueAsByteSlice("Vec<u8>", eventFields)
	if err != nil {
		return msg.Message{}, err
	}

	log.Info("Got fungible transfer event!", "destination", recipient, "resourceId", fmt.Sprintf("%x", resourceID), "amount", amount)

	return msg.NewFungibleTransfer(
		0, // Unset
		msg.ChainId(chainID),
		msg.Nonce(depositNonce),
		amount.Int,
		resourceID,
		recipient,
	), nil
}

func nonFungibleTransferHandler(_ registry.DecodedFields, log log15.Logger) (msg.Message, error) {
	log.Warn("Got non-fungible transfer event!")

	return msg.Message{}, errors.New("non-fungible transfer not supported")
}

func genericTransferHandler(eventFields registry.DecodedFields, log log15.Logger) (msg.Message, error) {
	chainID, err := getFieldValueAsType[types.U8]("ChainId", eventFields)
	if err != nil {
		return msg.Message{}, err
	}

	depositNonce, err := getFieldValueAsType[types.U64]("DepositNonce", eventFields)
	if err != nil {
		return msg.Message{}, err
	}

	resID, err := getFieldValueAsSliceOfType[types.U8]("ResourceId", eventFields)
	if err != nil {
		return msg.Message{}, err
	}

	resourceID, err := to32Bytes(resID)
	if err != nil {
		return msg.Message{}, err
	}

	metadata, err := getFieldValueAsByteSlice("Vec<u8>", eventFields)
	if err != nil {
		return msg.Message{}, err
	}

	log.Info("Got generic transfer event!", "destination", chainID, "resourceId", fmt.Sprintf("%x", resourceID))

	return msg.NewGenericTransfer(
		0, // Unset
		msg.ChainId(chainID),
		msg.Nonce(depositNonce),
		resourceID,
		metadata,
	), nil
}

func to32Bytes(array []types.U8) ([32]byte, error) {
	var res [32]byte

	if len(array) != 32 {
		return res, errors.New("array length mismatch")
	}

	for i, item := range array {
		res[i] = byte(item)
	}

	return res, nil
}

func getFieldValueAsType[T any](fieldName string, eventFields registry.DecodedFields) (T, error) {
	var t T

	for _, field := range eventFields {
		if field.Name == fieldName {
			if v, ok := field.Value.(T); ok {
				return v, nil
			}

			return t, fmt.Errorf("field type mismatch, expected %T, got %T", t, field.Value)
		}
	}

	return t, fmt.Errorf("field with name '%s' not found", fieldName)
}

func getFieldValueAsSliceOfType[T any](fieldName string, eventFields registry.DecodedFields) ([]T, error) {
	for _, field := range eventFields {
		if field.Name == fieldName {
			value, ok := field.Value.([]any)

			if !ok {
				return nil, errors.New("field value not an array")
			}

			res, err := convertSliceToType[T](value)

			if err != nil {
				return nil, err
			}

			return res, nil
		}
	}

	return nil, fmt.Errorf("field with name '%s' not found", fieldName)
}

func getFieldValueAsByteSlice(fieldName string, eventFields registry.DecodedFields) ([]byte, error) {
	for _, field := range eventFields {
		if field.Name == fieldName {
			value, ok := field.Value.([]any)

			if !ok {
				return nil, errors.New("field value not an array")
			}

			slice, err := convertSliceToType[types.U8](value)

			if err != nil {
				return nil, err
			}

			res, err := convertToByteSlice(slice)

			if err != nil {
				return nil, err
			}

			return res, nil
		}
	}

	return nil, fmt.Errorf("field with name '%s' not found", fieldName)
}

func convertSliceToType[T any](array []any) ([]T, error) {
	res := make([]T, 0)

	for _, item := range array {
		if v, ok := item.(T); ok {
			res = append(res, v)
			continue
		}

		var t T

		return nil, fmt.Errorf("couldn't cast '%T' to '%T'", item, t)
	}

	return res, nil
}

func convertToByteSlice(array []types.U8) ([]byte, error) {
	res := make([]byte, 0)

	for _, item := range array {
		res = append(res, byte(item))
	}

	return res, nil
}
