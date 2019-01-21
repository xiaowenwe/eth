// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package main

import (
	"eth/common"
	"levedbAndNeo4j/accounts/abi"
	"levedbAndNeo4j/core/types"
	"math/big"
	"strings"

	"github.com/ethereum/go-bjchain/accounts/abi/bind"
	"github.com/ethereum/go-bjchain/event"
	ethereum "github.com/ethereum/go-ethereum"
)

// GodTokenABI is the input ABI used to generate the binding from.
const GodTokenABI = "[{\"constant\":true,\"inputs\":[{\"name\":\"_customerAddress\",\"type\":\"address\"}],\"name\":\"dividendsOf\",\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"value\":\"0\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"name\",\"outputs\":[{\"name\":\"\",\"type\":\"string\",\"value\":\"God\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"name\":\"_spender\",\"type\":\"address\"},{\"name\":\"_value\",\"type\":\"uint256\"}],\"name\":\"approve\",\"outputs\":[{\"name\":\"success\",\"type\":\"bool\"}],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[{\"name\":\"_ethereumToSpend\",\"type\":\"uint256\"}],\"name\":\"calculateTokensReceived\",\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"value\":\"0\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"getContractPayout\",\"outputs\":[{\"name\":\"\",\"type\":\"int256\",\"value\":\"0\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"totalSupply\",\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"value\":\"0\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"name\":\"_from\",\"type\":\"address\"},{\"name\":\"_to\",\"type\":\"address\"},{\"name\":\"_amountOfTokens\",\"type\":\"uint256\"},{\"name\":\"_data\",\"type\":\"bytes\"}],\"name\":\"transferTo\",\"outputs\":[],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"getProfitPerShare\",\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"value\":\"0\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[{\"name\":\"_tokensToSell\",\"type\":\"uint256\"}],\"name\":\"calculateEthereumReceived\",\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"value\":\"0\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"name\":\"_from\",\"type\":\"address\"},{\"name\":\"_toAddress\",\"type\":\"address\"},{\"name\":\"_amountOfTokens\",\"type\":\"uint256\"}],\"name\":\"transferFrom\",\"outputs\":[{\"name\":\"\",\"type\":\"bool\"}],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"decimals\",\"outputs\":[{\"name\":\"\",\"type\":\"uint8\",\"value\":\"18\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[],\"name\":\"withdraw\",\"outputs\":[],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[{\"name\":\"\",\"type\":\"address\"}],\"name\":\"contractAddresses\",\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"value\":false}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"sellPrice\",\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"value\":\"90000000000\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"stakingRequirement\",\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"value\":\"100000000000000000000\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"name\":\"to\",\"type\":\"address\"},{\"name\":\"value\",\"type\":\"uint256\"}],\"name\":\"takeProjectBonus\",\"outputs\":[],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[{\"name\":\"_includeReferralBonus\",\"type\":\"bool\"}],\"name\":\"myDividends\",\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"value\":\"0\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[],\"name\":\"injectEther\",\"outputs\":[],\"payable\":true,\"stateMutability\":\"payable\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"totalEthereumBalance\",\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"value\":\"0\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[{\"name\":\"_customerAddress\",\"type\":\"address\"}],\"name\":\"balanceOf\",\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"value\":\"0\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[{\"name\":\"\",\"type\":\"address\"}],\"name\":\"administrators\",\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"value\":false}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"getIsProjectBonus\",\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"value\":false}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"name\":\"value\",\"type\":\"bool\"}],\"name\":\"setIsProjectBonus\",\"outputs\":[],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"getContractETH\",\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"value\":\"0\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"name\":\"_amountOfTokens\",\"type\":\"uint256\"}],\"name\":\"setStakingRequirement\",\"outputs\":[],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"buyPrice\",\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"value\":\"110000000000\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"name\":\"_identifier\",\"type\":\"address\"},{\"name\":\"_status\",\"type\":\"bool\"}],\"name\":\"setAdministrator\",\"outputs\":[],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[],\"name\":\"constructor\",\"outputs\":[],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"myTokens\",\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"value\":\"0\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"symbol\",\"outputs\":[{\"name\":\"\",\"type\":\"string\",\"value\":\"God\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[],\"name\":\"injectEtherToDividend\",\"outputs\":[],\"payable\":true,\"stateMutability\":\"payable\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"name\":\"_toAddress\",\"type\":\"address\"},{\"name\":\"_amountOfTokens\",\"type\":\"uint256\"}],\"name\":\"transfer\",\"outputs\":[{\"name\":\"\",\"type\":\"bool\"}],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[],\"name\":\"injectEtherFromIco\",\"outputs\":[],\"payable\":true,\"stateMutability\":\"payable\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[],\"name\":\"getProjectBonus\",\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"value\":\"0\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"name\":\"_symbol\",\"type\":\"string\"}],\"name\":\"setSymbol\",\"outputs\":[],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"name\":\"_identifier\",\"type\":\"address\"},{\"name\":\"value\",\"type\":\"uint256\"}],\"name\":\"setBank\",\"outputs\":[],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"name\":\"_name\",\"type\":\"string\"}],\"name\":\"setName\",\"outputs\":[],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[{\"name\":\"_owner\",\"type\":\"address\"},{\"name\":\"_spender\",\"type\":\"address\"}],\"name\":\"allowance\",\"outputs\":[{\"name\":\"remaining\",\"type\":\"uint256\",\"value\":\"0\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"name\":\"_amountOfTokens\",\"type\":\"uint256\"}],\"name\":\"sell\",\"outputs\":[],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[],\"name\":\"exit\",\"outputs\":[],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[{\"name\":\"_referredBy\",\"type\":\"address\"}],\"name\":\"buy\",\"outputs\":[{\"name\":\"\",\"type\":\"uint256\"}],\"payable\":true,\"stateMutability\":\"payable\",\"type\":\"function\"},{\"constant\":true,\"inputs\":[{\"name\":\"_customerAddress\",\"type\":\"address\"}],\"name\":\"getBalance\",\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"value\":\"0\"}],\"payable\":false,\"stateMutability\":\"view\",\"type\":\"function\"},{\"constant\":false,\"inputs\":[],\"name\":\"reinvest\",\"outputs\":[],\"payable\":false,\"stateMutability\":\"nonpayable\",\"type\":\"function\"},{\"payable\":true,\"stateMutability\":\"payable\",\"type\":\"fallback\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"name\":\"customerAddress\",\"type\":\"address\"},{\"indexed\":false,\"name\":\"incomingEthereum\",\"type\":\"uint256\"},{\"indexed\":false,\"name\":\"tokensMinted\",\"type\":\"uint256\"},{\"indexed\":true,\"name\":\"referredBy\",\"type\":\"address\"}],\"name\":\"onTokenPurchase\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"name\":\"customerAddress\",\"type\":\"address\"},{\"indexed\":false,\"name\":\"tokensBurned\",\"type\":\"uint256\"},{\"indexed\":false,\"name\":\"ethereumEarned\",\"type\":\"uint256\"}],\"name\":\"onTokenSell\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"name\":\"customerAddress\",\"type\":\"address\"},{\"indexed\":false,\"name\":\"ethereumReinvested\",\"type\":\"uint256\"},{\"indexed\":false,\"name\":\"tokensMinted\",\"type\":\"uint256\"}],\"name\":\"onReinvestment\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"name\":\"customerAddress\",\"type\":\"address\"},{\"indexed\":false,\"name\":\"ethereumWithdrawn\",\"type\":\"uint256\"}],\"name\":\"onWithdraw\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"name\":\"_incomingEthereum\",\"type\":\"uint256\"},{\"indexed\":false,\"name\":\"_dividends\",\"type\":\"uint256\"},{\"indexed\":false,\"name\":\"profitPerShare_\",\"type\":\"uint256\"}],\"name\":\"onInjectEtherFromIco\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":false,\"name\":\"sender\",\"type\":\"address\"},{\"indexed\":false,\"name\":\"_incomingEthereum\",\"type\":\"uint256\"},{\"indexed\":false,\"name\":\"profitPerShare_\",\"type\":\"uint256\"}],\"name\":\"onInjectEtherToDividend\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"name\":\"from\",\"type\":\"address\"},{\"indexed\":true,\"name\":\"to\",\"type\":\"address\"},{\"indexed\":false,\"name\":\"tokens\",\"type\":\"uint256\"}],\"name\":\"Transfer\",\"type\":\"event\"},{\"anonymous\":false,\"inputs\":[{\"indexed\":true,\"name\":\"_owner\",\"type\":\"address\"},{\"indexed\":true,\"name\":\"_spender\",\"type\":\"address\"},{\"indexed\":false,\"name\":\"_value\",\"type\":\"uint256\"}],\"name\":\"Approval\",\"type\":\"event\"}]"

// GodToken is an auto generated Go binding around an Ethereum contract.
type GodToken struct {
	GodTokenCaller     // Read-only binding to the contract
	GodTokenTransactor // Write-only binding to the contract
	GodTokenFilterer   // Log filterer for contract events
}

// GodTokenCaller is an auto generated read-only Go binding around an Ethereum contract.
type GodTokenCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// GodTokenTransactor is an auto generated write-only Go binding around an Ethereum contract.
type GodTokenTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// GodTokenFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type GodTokenFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// GodTokenSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type GodTokenSession struct {
	Contract     *GodToken         // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// GodTokenCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type GodTokenCallerSession struct {
	Contract *GodTokenCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts   // Call options to use throughout this session
}

// GodTokenTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type GodTokenTransactorSession struct {
	Contract     *GodTokenTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts   // Transaction auth options to use throughout this session
}

// GodTokenRaw is an auto generated low-level Go binding around an Ethereum contract.
type GodTokenRaw struct {
	Contract *GodToken // Generic contract binding to access the raw methods on
}

// GodTokenCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type GodTokenCallerRaw struct {
	Contract *GodTokenCaller // Generic read-only contract binding to access the raw methods on
}

// GodTokenTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type GodTokenTransactorRaw struct {
	Contract *GodTokenTransactor // Generic write-only contract binding to access the raw methods on
}

// NewGodToken creates a new instance of GodToken, bound to a specific deployed contract.
func NewGodToken(address common.Address, backend bind.ContractBackend) (*GodToken, error) {
	contract, err := bindGodToken(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &GodToken{GodTokenCaller: GodTokenCaller{contract: contract}, GodTokenTransactor: GodTokenTransactor{contract: contract}, GodTokenFilterer: GodTokenFilterer{contract: contract}}, nil
}

// NewGodTokenCaller creates a new read-only instance of GodToken, bound to a specific deployed contract.
func NewGodTokenCaller(address common.Address, caller bind.ContractCaller) (*GodTokenCaller, error) {
	contract, err := bindGodToken(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &GodTokenCaller{contract: contract}, nil
}

// NewGodTokenTransactor creates a new write-only instance of GodToken, bound to a specific deployed contract.
func NewGodTokenTransactor(address common.Address, transactor bind.ContractTransactor) (*GodTokenTransactor, error) {
	contract, err := bindGodToken(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &GodTokenTransactor{contract: contract}, nil
}

// NewGodTokenFilterer creates a new log filterer instance of GodToken, bound to a specific deployed contract.
func NewGodTokenFilterer(address common.Address, filterer bind.ContractFilterer) (*GodTokenFilterer, error) {
	contract, err := bindGodToken(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &GodTokenFilterer{contract: contract}, nil
}

// bindGodToken binds a generic wrapper to an already deployed contract.
func bindGodToken(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := abi.JSON(strings.NewReader(GodTokenABI))
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_GodToken *GodTokenRaw) Call(opts *bind.CallOpts, result interface{}, method string, params ...interface{}) error {
	return _GodToken.Contract.GodTokenCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_GodToken *GodTokenRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _GodToken.Contract.GodTokenTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_GodToken *GodTokenRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _GodToken.Contract.GodTokenTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_GodToken *GodTokenCallerRaw) Call(opts *bind.CallOpts, result interface{}, method string, params ...interface{}) error {
	return _GodToken.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_GodToken *GodTokenTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _GodToken.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_GodToken *GodTokenTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _GodToken.Contract.contract.Transact(opts, method, params...)
}

// Administrators is a free data retrieval call binding the contract method 0x76be1585.
//
// Solidity: function administrators( address) constant returns(bool)
func (_GodToken *GodTokenCaller) Administrators(opts *bind.CallOpts, arg0 common.Address) (bool, error) {
	var (
		ret0 = new(bool)
	)
	out := ret0
	err := _GodToken.contract.Call(opts, out, "administrators", arg0)
	return *ret0, err
}

// Administrators is a free data retrieval call binding the contract method 0x76be1585.
//
// Solidity: function administrators( address) constant returns(bool)
func (_GodToken *GodTokenSession) Administrators(arg0 common.Address) (bool, error) {
	return _GodToken.Contract.Administrators(&_GodToken.CallOpts, arg0)
}

// Administrators is a free data retrieval call binding the contract method 0x76be1585.
//
// Solidity: function administrators( address) constant returns(bool)
func (_GodToken *GodTokenCallerSession) Administrators(arg0 common.Address) (bool, error) {
	return _GodToken.Contract.Administrators(&_GodToken.CallOpts, arg0)
}

// Allowance is a free data retrieval call binding the contract method 0xdd62ed3e.
//
// Solidity: function allowance(_owner address, _spender address) constant returns(remaining uint256)
func (_GodToken *GodTokenCaller) Allowance(opts *bind.CallOpts, _owner common.Address, _spender common.Address) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _GodToken.contract.Call(opts, out, "allowance", _owner, _spender)
	return *ret0, err
}

// Allowance is a free data retrieval call binding the contract method 0xdd62ed3e.
//
// Solidity: function allowance(_owner address, _spender address) constant returns(remaining uint256)
func (_GodToken *GodTokenSession) Allowance(_owner common.Address, _spender common.Address) (*big.Int, error) {
	return _GodToken.Contract.Allowance(&_GodToken.CallOpts, _owner, _spender)
}

// Allowance is a free data retrieval call binding the contract method 0xdd62ed3e.
//
// Solidity: function allowance(_owner address, _spender address) constant returns(remaining uint256)
func (_GodToken *GodTokenCallerSession) Allowance(_owner common.Address, _spender common.Address) (*big.Int, error) {
	return _GodToken.Contract.Allowance(&_GodToken.CallOpts, _owner, _spender)
}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(_customerAddress address) constant returns(uint256)
func (_GodToken *GodTokenCaller) BalanceOf(opts *bind.CallOpts, _customerAddress common.Address) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _GodToken.contract.Call(opts, out, "balanceOf", _customerAddress)
	return *ret0, err
}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(_customerAddress address) constant returns(uint256)
func (_GodToken *GodTokenSession) BalanceOf(_customerAddress common.Address) (*big.Int, error) {
	return _GodToken.Contract.BalanceOf(&_GodToken.CallOpts, _customerAddress)
}

// BalanceOf is a free data retrieval call binding the contract method 0x70a08231.
//
// Solidity: function balanceOf(_customerAddress address) constant returns(uint256)
func (_GodToken *GodTokenCallerSession) BalanceOf(_customerAddress common.Address) (*big.Int, error) {
	return _GodToken.Contract.BalanceOf(&_GodToken.CallOpts, _customerAddress)
}

// BuyPrice is a free data retrieval call binding the contract method 0x8620410b.
//
// Solidity: function buyPrice() constant returns(uint256)
func (_GodToken *GodTokenCaller) BuyPrice(opts *bind.CallOpts) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _GodToken.contract.Call(opts, out, "buyPrice")
	return *ret0, err
}

// BuyPrice is a free data retrieval call binding the contract method 0x8620410b.
//
// Solidity: function buyPrice() constant returns(uint256)
func (_GodToken *GodTokenSession) BuyPrice() (*big.Int, error) {
	return _GodToken.Contract.BuyPrice(&_GodToken.CallOpts)
}

// BuyPrice is a free data retrieval call binding the contract method 0x8620410b.
//
// Solidity: function buyPrice() constant returns(uint256)
func (_GodToken *GodTokenCallerSession) BuyPrice() (*big.Int, error) {
	return _GodToken.Contract.BuyPrice(&_GodToken.CallOpts)
}

// CalculateEthereumReceived is a free data retrieval call binding the contract method 0x22609373.
//
// Solidity: function calculateEthereumReceived(_tokensToSell uint256) constant returns(uint256)
func (_GodToken *GodTokenCaller) CalculateEthereumReceived(opts *bind.CallOpts, _tokensToSell *big.Int) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _GodToken.contract.Call(opts, out, "calculateEthereumReceived", _tokensToSell)
	return *ret0, err
}

// CalculateEthereumReceived is a free data retrieval call binding the contract method 0x22609373.
//
// Solidity: function calculateEthereumReceived(_tokensToSell uint256) constant returns(uint256)
func (_GodToken *GodTokenSession) CalculateEthereumReceived(_tokensToSell *big.Int) (*big.Int, error) {
	return _GodToken.Contract.CalculateEthereumReceived(&_GodToken.CallOpts, _tokensToSell)
}

// CalculateEthereumReceived is a free data retrieval call binding the contract method 0x22609373.
//
// Solidity: function calculateEthereumReceived(_tokensToSell uint256) constant returns(uint256)
func (_GodToken *GodTokenCallerSession) CalculateEthereumReceived(_tokensToSell *big.Int) (*big.Int, error) {
	return _GodToken.Contract.CalculateEthereumReceived(&_GodToken.CallOpts, _tokensToSell)
}

// CalculateTokensReceived is a free data retrieval call binding the contract method 0x10d0ffdd.
//
// Solidity: function calculateTokensReceived(_ethereumToSpend uint256) constant returns(uint256)
func (_GodToken *GodTokenCaller) CalculateTokensReceived(opts *bind.CallOpts, _ethereumToSpend *big.Int) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _GodToken.contract.Call(opts, out, "calculateTokensReceived", _ethereumToSpend)
	return *ret0, err
}

// CalculateTokensReceived is a free data retrieval call binding the contract method 0x10d0ffdd.
//
// Solidity: function calculateTokensReceived(_ethereumToSpend uint256) constant returns(uint256)
func (_GodToken *GodTokenSession) CalculateTokensReceived(_ethereumToSpend *big.Int) (*big.Int, error) {
	return _GodToken.Contract.CalculateTokensReceived(&_GodToken.CallOpts, _ethereumToSpend)
}

// CalculateTokensReceived is a free data retrieval call binding the contract method 0x10d0ffdd.
//
// Solidity: function calculateTokensReceived(_ethereumToSpend uint256) constant returns(uint256)
func (_GodToken *GodTokenCallerSession) CalculateTokensReceived(_ethereumToSpend *big.Int) (*big.Int, error) {
	return _GodToken.Contract.CalculateTokensReceived(&_GodToken.CallOpts, _ethereumToSpend)
}

// ContractAddresses is a free data retrieval call binding the contract method 0x4661ac95.
//
// Solidity: function contractAddresses( address) constant returns(bool)
func (_GodToken *GodTokenCaller) ContractAddresses(opts *bind.CallOpts, arg0 common.Address) (bool, error) {
	var (
		ret0 = new(bool)
	)
	out := ret0
	err := _GodToken.contract.Call(opts, out, "contractAddresses", arg0)
	return *ret0, err
}

// ContractAddresses is a free data retrieval call binding the contract method 0x4661ac95.
//
// Solidity: function contractAddresses( address) constant returns(bool)
func (_GodToken *GodTokenSession) ContractAddresses(arg0 common.Address) (bool, error) {
	return _GodToken.Contract.ContractAddresses(&_GodToken.CallOpts, arg0)
}

// ContractAddresses is a free data retrieval call binding the contract method 0x4661ac95.
//
// Solidity: function contractAddresses( address) constant returns(bool)
func (_GodToken *GodTokenCallerSession) ContractAddresses(arg0 common.Address) (bool, error) {
	return _GodToken.Contract.ContractAddresses(&_GodToken.CallOpts, arg0)
}

// Decimals is a free data retrieval call binding the contract method 0x313ce567.
//
// Solidity: function decimals() constant returns(uint8)
func (_GodToken *GodTokenCaller) Decimals(opts *bind.CallOpts) (uint8, error) {
	var (
		ret0 = new(uint8)
	)
	out := ret0
	err := _GodToken.contract.Call(opts, out, "decimals")
	return *ret0, err
}

// Decimals is a free data retrieval call binding the contract method 0x313ce567.
//
// Solidity: function decimals() constant returns(uint8)
func (_GodToken *GodTokenSession) Decimals() (uint8, error) {
	return _GodToken.Contract.Decimals(&_GodToken.CallOpts)
}

// Decimals is a free data retrieval call binding the contract method 0x313ce567.
//
// Solidity: function decimals() constant returns(uint8)
func (_GodToken *GodTokenCallerSession) Decimals() (uint8, error) {
	return _GodToken.Contract.Decimals(&_GodToken.CallOpts)
}

// DividendsOf is a free data retrieval call binding the contract method 0x0065318b.
//
// Solidity: function dividendsOf(_customerAddress address) constant returns(uint256)
func (_GodToken *GodTokenCaller) DividendsOf(opts *bind.CallOpts, _customerAddress common.Address) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _GodToken.contract.Call(opts, out, "dividendsOf", _customerAddress)
	return *ret0, err
}

// DividendsOf is a free data retrieval call binding the contract method 0x0065318b.
//
// Solidity: function dividendsOf(_customerAddress address) constant returns(uint256)
func (_GodToken *GodTokenSession) DividendsOf(_customerAddress common.Address) (*big.Int, error) {
	return _GodToken.Contract.DividendsOf(&_GodToken.CallOpts, _customerAddress)
}

// DividendsOf is a free data retrieval call binding the contract method 0x0065318b.
//
// Solidity: function dividendsOf(_customerAddress address) constant returns(uint256)
func (_GodToken *GodTokenCallerSession) DividendsOf(_customerAddress common.Address) (*big.Int, error) {
	return _GodToken.Contract.DividendsOf(&_GodToken.CallOpts, _customerAddress)
}

// GetBalance is a free data retrieval call binding the contract method 0xf8b2cb4f.
//
// Solidity: function getBalance(_customerAddress address) constant returns(uint256)
func (_GodToken *GodTokenCaller) GetBalance(opts *bind.CallOpts, _customerAddress common.Address) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _GodToken.contract.Call(opts, out, "getBalance", _customerAddress)
	return *ret0, err
}

// GetBalance is a free data retrieval call binding the contract method 0xf8b2cb4f.
//
// Solidity: function getBalance(_customerAddress address) constant returns(uint256)
func (_GodToken *GodTokenSession) GetBalance(_customerAddress common.Address) (*big.Int, error) {
	return _GodToken.Contract.GetBalance(&_GodToken.CallOpts, _customerAddress)
}

// GetBalance is a free data retrieval call binding the contract method 0xf8b2cb4f.
//
// Solidity: function getBalance(_customerAddress address) constant returns(uint256)
func (_GodToken *GodTokenCallerSession) GetBalance(_customerAddress common.Address) (*big.Int, error) {
	return _GodToken.Contract.GetBalance(&_GodToken.CallOpts, _customerAddress)
}

// GetContractETH is a free data retrieval call binding the contract method 0x8258cbbd.
//
// Solidity: function getContractETH() constant returns(uint256)
func (_GodToken *GodTokenCaller) GetContractETH(opts *bind.CallOpts) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _GodToken.contract.Call(opts, out, "getContractETH")
	return *ret0, err
}

// GetContractETH is a free data retrieval call binding the contract method 0x8258cbbd.
//
// Solidity: function getContractETH() constant returns(uint256)
func (_GodToken *GodTokenSession) GetContractETH() (*big.Int, error) {
	return _GodToken.Contract.GetContractETH(&_GodToken.CallOpts)
}

// GetContractETH is a free data retrieval call binding the contract method 0x8258cbbd.
//
// Solidity: function getContractETH() constant returns(uint256)
func (_GodToken *GodTokenCallerSession) GetContractETH() (*big.Int, error) {
	return _GodToken.Contract.GetContractETH(&_GodToken.CallOpts)
}

// GetContractPayout is a free data retrieval call binding the contract method 0x1741526f.
//
// Solidity: function getContractPayout() constant returns(int256)
func (_GodToken *GodTokenCaller) GetContractPayout(opts *bind.CallOpts) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _GodToken.contract.Call(opts, out, "getContractPayout")
	return *ret0, err
}

// GetContractPayout is a free data retrieval call binding the contract method 0x1741526f.
//
// Solidity: function getContractPayout() constant returns(int256)
func (_GodToken *GodTokenSession) GetContractPayout() (*big.Int, error) {
	return _GodToken.Contract.GetContractPayout(&_GodToken.CallOpts)
}

// GetContractPayout is a free data retrieval call binding the contract method 0x1741526f.
//
// Solidity: function getContractPayout() constant returns(int256)
func (_GodToken *GodTokenCallerSession) GetContractPayout() (*big.Int, error) {
	return _GodToken.Contract.GetContractPayout(&_GodToken.CallOpts)
}

// GetIsProjectBonus is a free data retrieval call binding the contract method 0x7c71c0eb.
//
// Solidity: function getIsProjectBonus() constant returns(bool)
func (_GodToken *GodTokenCaller) GetIsProjectBonus(opts *bind.CallOpts) (bool, error) {
	var (
		ret0 = new(bool)
	)
	out := ret0
	err := _GodToken.contract.Call(opts, out, "getIsProjectBonus")
	return *ret0, err
}

// GetIsProjectBonus is a free data retrieval call binding the contract method 0x7c71c0eb.
//
// Solidity: function getIsProjectBonus() constant returns(bool)
func (_GodToken *GodTokenSession) GetIsProjectBonus() (bool, error) {
	return _GodToken.Contract.GetIsProjectBonus(&_GodToken.CallOpts)
}

// GetIsProjectBonus is a free data retrieval call binding the contract method 0x7c71c0eb.
//
// Solidity: function getIsProjectBonus() constant returns(bool)
func (_GodToken *GodTokenCallerSession) GetIsProjectBonus() (bool, error) {
	return _GodToken.Contract.GetIsProjectBonus(&_GodToken.CallOpts)
}

// GetProfitPerShare is a free data retrieval call binding the contract method 0x1aebcb89.
//
// Solidity: function getProfitPerShare() constant returns(uint256)
func (_GodToken *GodTokenCaller) GetProfitPerShare(opts *bind.CallOpts) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _GodToken.contract.Call(opts, out, "getProfitPerShare")
	return *ret0, err
}

// GetProfitPerShare is a free data retrieval call binding the contract method 0x1aebcb89.
//
// Solidity: function getProfitPerShare() constant returns(uint256)
func (_GodToken *GodTokenSession) GetProfitPerShare() (*big.Int, error) {
	return _GodToken.Contract.GetProfitPerShare(&_GodToken.CallOpts)
}

// GetProfitPerShare is a free data retrieval call binding the contract method 0x1aebcb89.
//
// Solidity: function getProfitPerShare() constant returns(uint256)
func (_GodToken *GodTokenCallerSession) GetProfitPerShare() (*big.Int, error) {
	return _GodToken.Contract.GetProfitPerShare(&_GodToken.CallOpts)
}

// GetProjectBonus is a free data retrieval call binding the contract method 0xb7b5e811.
//
// Solidity: function getProjectBonus() constant returns(uint256)
func (_GodToken *GodTokenCaller) GetProjectBonus(opts *bind.CallOpts) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _GodToken.contract.Call(opts, out, "getProjectBonus")
	return *ret0, err
}

// GetProjectBonus is a free data retrieval call binding the contract method 0xb7b5e811.
//
// Solidity: function getProjectBonus() constant returns(uint256)
func (_GodToken *GodTokenSession) GetProjectBonus() (*big.Int, error) {
	return _GodToken.Contract.GetProjectBonus(&_GodToken.CallOpts)
}

// GetProjectBonus is a free data retrieval call binding the contract method 0xb7b5e811.
//
// Solidity: function getProjectBonus() constant returns(uint256)
func (_GodToken *GodTokenCallerSession) GetProjectBonus() (*big.Int, error) {
	return _GodToken.Contract.GetProjectBonus(&_GodToken.CallOpts)
}

// MyDividends is a free data retrieval call binding the contract method 0x688abbf7.
//
// Solidity: function myDividends(_includeReferralBonus bool) constant returns(uint256)
func (_GodToken *GodTokenCaller) MyDividends(opts *bind.CallOpts, _includeReferralBonus bool) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _GodToken.contract.Call(opts, out, "myDividends", _includeReferralBonus)
	return *ret0, err
}

// MyDividends is a free data retrieval call binding the contract method 0x688abbf7.
//
// Solidity: function myDividends(_includeReferralBonus bool) constant returns(uint256)
func (_GodToken *GodTokenSession) MyDividends(_includeReferralBonus bool) (*big.Int, error) {
	return _GodToken.Contract.MyDividends(&_GodToken.CallOpts, _includeReferralBonus)
}

// MyDividends is a free data retrieval call binding the contract method 0x688abbf7.
//
// Solidity: function myDividends(_includeReferralBonus bool) constant returns(uint256)
func (_GodToken *GodTokenCallerSession) MyDividends(_includeReferralBonus bool) (*big.Int, error) {
	return _GodToken.Contract.MyDividends(&_GodToken.CallOpts, _includeReferralBonus)
}

// MyTokens is a free data retrieval call binding the contract method 0x949e8acd.
//
// Solidity: function myTokens() constant returns(uint256)
func (_GodToken *GodTokenCaller) MyTokens(opts *bind.CallOpts) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _GodToken.contract.Call(opts, out, "myTokens")
	return *ret0, err
}

// MyTokens is a free data retrieval call binding the contract method 0x949e8acd.
//
// Solidity: function myTokens() constant returns(uint256)
func (_GodToken *GodTokenSession) MyTokens() (*big.Int, error) {
	return _GodToken.Contract.MyTokens(&_GodToken.CallOpts)
}

// MyTokens is a free data retrieval call binding the contract method 0x949e8acd.
//
// Solidity: function myTokens() constant returns(uint256)
func (_GodToken *GodTokenCallerSession) MyTokens() (*big.Int, error) {
	return _GodToken.Contract.MyTokens(&_GodToken.CallOpts)
}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() constant returns(string)
func (_GodToken *GodTokenCaller) Name(opts *bind.CallOpts) (string, error) {
	var (
		ret0 = new(string)
	)
	out := ret0
	err := _GodToken.contract.Call(opts, out, "name")
	return *ret0, err
}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() constant returns(string)
func (_GodToken *GodTokenSession) Name() (string, error) {
	return _GodToken.Contract.Name(&_GodToken.CallOpts)
}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() constant returns(string)
func (_GodToken *GodTokenCallerSession) Name() (string, error) {
	return _GodToken.Contract.Name(&_GodToken.CallOpts)
}

// SellPrice is a free data retrieval call binding the contract method 0x4b750334.
//
// Solidity: function sellPrice() constant returns(uint256)
func (_GodToken *GodTokenCaller) SellPrice(opts *bind.CallOpts) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _GodToken.contract.Call(opts, out, "sellPrice")
	return *ret0, err
}

// SellPrice is a free data retrieval call binding the contract method 0x4b750334.
//
// Solidity: function sellPrice() constant returns(uint256)
func (_GodToken *GodTokenSession) SellPrice() (*big.Int, error) {
	return _GodToken.Contract.SellPrice(&_GodToken.CallOpts)
}

// SellPrice is a free data retrieval call binding the contract method 0x4b750334.
//
// Solidity: function sellPrice() constant returns(uint256)
func (_GodToken *GodTokenCallerSession) SellPrice() (*big.Int, error) {
	return _GodToken.Contract.SellPrice(&_GodToken.CallOpts)
}

// StakingRequirement is a free data retrieval call binding the contract method 0x56d399e8.
//
// Solidity: function stakingRequirement() constant returns(uint256)
func (_GodToken *GodTokenCaller) StakingRequirement(opts *bind.CallOpts) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _GodToken.contract.Call(opts, out, "stakingRequirement")
	return *ret0, err
}

// StakingRequirement is a free data retrieval call binding the contract method 0x56d399e8.
//
// Solidity: function stakingRequirement() constant returns(uint256)
func (_GodToken *GodTokenSession) StakingRequirement() (*big.Int, error) {
	return _GodToken.Contract.StakingRequirement(&_GodToken.CallOpts)
}

// StakingRequirement is a free data retrieval call binding the contract method 0x56d399e8.
//
// Solidity: function stakingRequirement() constant returns(uint256)
func (_GodToken *GodTokenCallerSession) StakingRequirement() (*big.Int, error) {
	return _GodToken.Contract.StakingRequirement(&_GodToken.CallOpts)
}

// Symbol is a free data retrieval call binding the contract method 0x95d89b41.
//
// Solidity: function symbol() constant returns(string)
func (_GodToken *GodTokenCaller) Symbol(opts *bind.CallOpts) (string, error) {
	var (
		ret0 = new(string)
	)
	out := ret0
	err := _GodToken.contract.Call(opts, out, "symbol")
	return *ret0, err
}

// Symbol is a free data retrieval call binding the contract method 0x95d89b41.
//
// Solidity: function symbol() constant returns(string)
func (_GodToken *GodTokenSession) Symbol() (string, error) {
	return _GodToken.Contract.Symbol(&_GodToken.CallOpts)
}

// Symbol is a free data retrieval call binding the contract method 0x95d89b41.
//
// Solidity: function symbol() constant returns(string)
func (_GodToken *GodTokenCallerSession) Symbol() (string, error) {
	return _GodToken.Contract.Symbol(&_GodToken.CallOpts)
}

// TotalEthereumBalance is a free data retrieval call binding the contract method 0x6b2f4632.
//
// Solidity: function totalEthereumBalance() constant returns(uint256)
func (_GodToken *GodTokenCaller) TotalEthereumBalance(opts *bind.CallOpts) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _GodToken.contract.Call(opts, out, "totalEthereumBalance")
	return *ret0, err
}

// TotalEthereumBalance is a free data retrieval call binding the contract method 0x6b2f4632.
//
// Solidity: function totalEthereumBalance() constant returns(uint256)
func (_GodToken *GodTokenSession) TotalEthereumBalance() (*big.Int, error) {
	return _GodToken.Contract.TotalEthereumBalance(&_GodToken.CallOpts)
}

// TotalEthereumBalance is a free data retrieval call binding the contract method 0x6b2f4632.
//
// Solidity: function totalEthereumBalance() constant returns(uint256)
func (_GodToken *GodTokenCallerSession) TotalEthereumBalance() (*big.Int, error) {
	return _GodToken.Contract.TotalEthereumBalance(&_GodToken.CallOpts)
}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() constant returns(uint256)
func (_GodToken *GodTokenCaller) TotalSupply(opts *bind.CallOpts) (*big.Int, error) {
	var (
		ret0 = new(*big.Int)
	)
	out := ret0
	err := _GodToken.contract.Call(opts, out, "totalSupply")
	return *ret0, err
}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() constant returns(uint256)
func (_GodToken *GodTokenSession) TotalSupply() (*big.Int, error) {
	return _GodToken.Contract.TotalSupply(&_GodToken.CallOpts)
}

// TotalSupply is a free data retrieval call binding the contract method 0x18160ddd.
//
// Solidity: function totalSupply() constant returns(uint256)
func (_GodToken *GodTokenCallerSession) TotalSupply() (*big.Int, error) {
	return _GodToken.Contract.TotalSupply(&_GodToken.CallOpts)
}

// Approve is a paid mutator transaction binding the contract method 0x095ea7b3.
//
// Solidity: function approve(_spender address, _value uint256) returns(success bool)
func (_GodToken *GodTokenTransactor) Approve(opts *bind.TransactOpts, _spender common.Address, _value *big.Int) (*types.Transaction, error) {
	return _GodToken.contract.Transact(opts, "approve", _spender, _value)
}

// Approve is a paid mutator transaction binding the contract method 0x095ea7b3.
//
// Solidity: function approve(_spender address, _value uint256) returns(success bool)
func (_GodToken *GodTokenSession) Approve(_spender common.Address, _value *big.Int) (*types.Transaction, error) {
	return _GodToken.Contract.Approve(&_GodToken.TransactOpts, _spender, _value)
}

// Approve is a paid mutator transaction binding the contract method 0x095ea7b3.
//
// Solidity: function approve(_spender address, _value uint256) returns(success bool)
func (_GodToken *GodTokenTransactorSession) Approve(_spender common.Address, _value *big.Int) (*types.Transaction, error) {
	return _GodToken.Contract.Approve(&_GodToken.TransactOpts, _spender, _value)
}

// Buy is a paid mutator transaction binding the contract method 0xf088d547.
//
// Solidity: function buy(_referredBy address) returns(uint256)
func (_GodToken *GodTokenTransactor) Buy(opts *bind.TransactOpts, _referredBy common.Address) (*types.Transaction, error) {
	return _GodToken.contract.Transact(opts, "buy", _referredBy)
}

// Buy is a paid mutator transaction binding the contract method 0xf088d547.
//
// Solidity: function buy(_referredBy address) returns(uint256)
func (_GodToken *GodTokenSession) Buy(_referredBy common.Address) (*types.Transaction, error) {
	return _GodToken.Contract.Buy(&_GodToken.TransactOpts, _referredBy)
}

// Buy is a paid mutator transaction binding the contract method 0xf088d547.
//
// Solidity: function buy(_referredBy address) returns(uint256)
func (_GodToken *GodTokenTransactorSession) Buy(_referredBy common.Address) (*types.Transaction, error) {
	return _GodToken.Contract.Buy(&_GodToken.TransactOpts, _referredBy)
}

// Constructor is a paid mutator transaction binding the contract method 0x90fa17bb.
//
// Solidity: function constructor() returns()
func (_GodToken *GodTokenTransactor) Constructor(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _GodToken.contract.Transact(opts, "constructor")
}

// Constructor is a paid mutator transaction binding the contract method 0x90fa17bb.
//
// Solidity: function constructor() returns()
func (_GodToken *GodTokenSession) Constructor() (*types.Transaction, error) {
	return _GodToken.Contract.Constructor(&_GodToken.TransactOpts)
}

// Constructor is a paid mutator transaction binding the contract method 0x90fa17bb.
//
// Solidity: function constructor() returns()
func (_GodToken *GodTokenTransactorSession) Constructor() (*types.Transaction, error) {
	return _GodToken.Contract.Constructor(&_GodToken.TransactOpts)
}

// Exit is a paid mutator transaction binding the contract method 0xe9fad8ee.
//
// Solidity: function exit() returns()
func (_GodToken *GodTokenTransactor) Exit(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _GodToken.contract.Transact(opts, "exit")
}

// Exit is a paid mutator transaction binding the contract method 0xe9fad8ee.
//
// Solidity: function exit() returns()
func (_GodToken *GodTokenSession) Exit() (*types.Transaction, error) {
	return _GodToken.Contract.Exit(&_GodToken.TransactOpts)
}

// Exit is a paid mutator transaction binding the contract method 0xe9fad8ee.
//
// Solidity: function exit() returns()
func (_GodToken *GodTokenTransactorSession) Exit() (*types.Transaction, error) {
	return _GodToken.Contract.Exit(&_GodToken.TransactOpts)
}

// InjectEther is a paid mutator transaction binding the contract method 0x6a3a2119.
//
// Solidity: function injectEther() returns()
func (_GodToken *GodTokenTransactor) InjectEther(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _GodToken.contract.Transact(opts, "injectEther")
}

// InjectEther is a paid mutator transaction binding the contract method 0x6a3a2119.
//
// Solidity: function injectEther() returns()
func (_GodToken *GodTokenSession) InjectEther() (*types.Transaction, error) {
	return _GodToken.Contract.InjectEther(&_GodToken.TransactOpts)
}

// InjectEther is a paid mutator transaction binding the contract method 0x6a3a2119.
//
// Solidity: function injectEther() returns()
func (_GodToken *GodTokenTransactorSession) InjectEther() (*types.Transaction, error) {
	return _GodToken.Contract.InjectEther(&_GodToken.TransactOpts)
}

// InjectEtherFromIco is a paid mutator transaction binding the contract method 0xb67e2064.
//
// Solidity: function injectEtherFromIco() returns()
func (_GodToken *GodTokenTransactor) InjectEtherFromIco(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _GodToken.contract.Transact(opts, "injectEtherFromIco")
}

// InjectEtherFromIco is a paid mutator transaction binding the contract method 0xb67e2064.
//
// Solidity: function injectEtherFromIco() returns()
func (_GodToken *GodTokenSession) InjectEtherFromIco() (*types.Transaction, error) {
	return _GodToken.Contract.InjectEtherFromIco(&_GodToken.TransactOpts)
}

// InjectEtherFromIco is a paid mutator transaction binding the contract method 0xb67e2064.
//
// Solidity: function injectEtherFromIco() returns()
func (_GodToken *GodTokenTransactorSession) InjectEtherFromIco() (*types.Transaction, error) {
	return _GodToken.Contract.InjectEtherFromIco(&_GodToken.TransactOpts)
}

// InjectEtherToDividend is a paid mutator transaction binding the contract method 0xa6741cfd.
//
// Solidity: function injectEtherToDividend() returns()
func (_GodToken *GodTokenTransactor) InjectEtherToDividend(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _GodToken.contract.Transact(opts, "injectEtherToDividend")
}

// InjectEtherToDividend is a paid mutator transaction binding the contract method 0xa6741cfd.
//
// Solidity: function injectEtherToDividend() returns()
func (_GodToken *GodTokenSession) InjectEtherToDividend() (*types.Transaction, error) {
	return _GodToken.Contract.InjectEtherToDividend(&_GodToken.TransactOpts)
}

// InjectEtherToDividend is a paid mutator transaction binding the contract method 0xa6741cfd.
//
// Solidity: function injectEtherToDividend() returns()
func (_GodToken *GodTokenTransactorSession) InjectEtherToDividend() (*types.Transaction, error) {
	return _GodToken.Contract.InjectEtherToDividend(&_GodToken.TransactOpts)
}

// Reinvest is a paid mutator transaction binding the contract method 0xfdb5a03e.
//
// Solidity: function reinvest() returns()
func (_GodToken *GodTokenTransactor) Reinvest(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _GodToken.contract.Transact(opts, "reinvest")
}

// Reinvest is a paid mutator transaction binding the contract method 0xfdb5a03e.
//
// Solidity: function reinvest() returns()
func (_GodToken *GodTokenSession) Reinvest() (*types.Transaction, error) {
	return _GodToken.Contract.Reinvest(&_GodToken.TransactOpts)
}

// Reinvest is a paid mutator transaction binding the contract method 0xfdb5a03e.
//
// Solidity: function reinvest() returns()
func (_GodToken *GodTokenTransactorSession) Reinvest() (*types.Transaction, error) {
	return _GodToken.Contract.Reinvest(&_GodToken.TransactOpts)
}

// Sell is a paid mutator transaction binding the contract method 0xe4849b32.
//
// Solidity: function sell(_amountOfTokens uint256) returns()
func (_GodToken *GodTokenTransactor) Sell(opts *bind.TransactOpts, _amountOfTokens *big.Int) (*types.Transaction, error) {
	return _GodToken.contract.Transact(opts, "sell", _amountOfTokens)
}

// Sell is a paid mutator transaction binding the contract method 0xe4849b32.
//
// Solidity: function sell(_amountOfTokens uint256) returns()
func (_GodToken *GodTokenSession) Sell(_amountOfTokens *big.Int) (*types.Transaction, error) {
	return _GodToken.Contract.Sell(&_GodToken.TransactOpts, _amountOfTokens)
}

// Sell is a paid mutator transaction binding the contract method 0xe4849b32.
//
// Solidity: function sell(_amountOfTokens uint256) returns()
func (_GodToken *GodTokenTransactorSession) Sell(_amountOfTokens *big.Int) (*types.Transaction, error) {
	return _GodToken.Contract.Sell(&_GodToken.TransactOpts, _amountOfTokens)
}

// SetAdministrator is a paid mutator transaction binding the contract method 0x87c95058.
//
// Solidity: function setAdministrator(_identifier address, _status bool) returns()
func (_GodToken *GodTokenTransactor) SetAdministrator(opts *bind.TransactOpts, _identifier common.Address, _status bool) (*types.Transaction, error) {
	return _GodToken.contract.Transact(opts, "setAdministrator", _identifier, _status)
}

// SetAdministrator is a paid mutator transaction binding the contract method 0x87c95058.
//
// Solidity: function setAdministrator(_identifier address, _status bool) returns()
func (_GodToken *GodTokenSession) SetAdministrator(_identifier common.Address, _status bool) (*types.Transaction, error) {
	return _GodToken.Contract.SetAdministrator(&_GodToken.TransactOpts, _identifier, _status)
}

// SetAdministrator is a paid mutator transaction binding the contract method 0x87c95058.
//
// Solidity: function setAdministrator(_identifier address, _status bool) returns()
func (_GodToken *GodTokenTransactorSession) SetAdministrator(_identifier common.Address, _status bool) (*types.Transaction, error) {
	return _GodToken.Contract.SetAdministrator(&_GodToken.TransactOpts, _identifier, _status)
}

// SetBank is a paid mutator transaction binding the contract method 0xbc44ea9a.
//
// Solidity: function setBank(_identifier address, value uint256) returns()
func (_GodToken *GodTokenTransactor) SetBank(opts *bind.TransactOpts, _identifier common.Address, value *big.Int) (*types.Transaction, error) {
	return _GodToken.contract.Transact(opts, "setBank", _identifier, value)
}

// SetBank is a paid mutator transaction binding the contract method 0xbc44ea9a.
//
// Solidity: function setBank(_identifier address, value uint256) returns()
func (_GodToken *GodTokenSession) SetBank(_identifier common.Address, value *big.Int) (*types.Transaction, error) {
	return _GodToken.Contract.SetBank(&_GodToken.TransactOpts, _identifier, value)
}

// SetBank is a paid mutator transaction binding the contract method 0xbc44ea9a.
//
// Solidity: function setBank(_identifier address, value uint256) returns()
func (_GodToken *GodTokenTransactorSession) SetBank(_identifier common.Address, value *big.Int) (*types.Transaction, error) {
	return _GodToken.Contract.SetBank(&_GodToken.TransactOpts, _identifier, value)
}

// SetIsProjectBonus is a paid mutator transaction binding the contract method 0x80560a0a.
//
// Solidity: function setIsProjectBonus(value bool) returns()
func (_GodToken *GodTokenTransactor) SetIsProjectBonus(opts *bind.TransactOpts, value bool) (*types.Transaction, error) {
	return _GodToken.contract.Transact(opts, "setIsProjectBonus", value)
}

// SetIsProjectBonus is a paid mutator transaction binding the contract method 0x80560a0a.
//
// Solidity: function setIsProjectBonus(value bool) returns()
func (_GodToken *GodTokenSession) SetIsProjectBonus(value bool) (*types.Transaction, error) {
	return _GodToken.Contract.SetIsProjectBonus(&_GodToken.TransactOpts, value)
}

// SetIsProjectBonus is a paid mutator transaction binding the contract method 0x80560a0a.
//
// Solidity: function setIsProjectBonus(value bool) returns()
func (_GodToken *GodTokenTransactorSession) SetIsProjectBonus(value bool) (*types.Transaction, error) {
	return _GodToken.Contract.SetIsProjectBonus(&_GodToken.TransactOpts, value)
}

// SetName is a paid mutator transaction binding the contract method 0xc47f0027.
//
// Solidity: function setName(_name string) returns()
func (_GodToken *GodTokenTransactor) SetName(opts *bind.TransactOpts, _name string) (*types.Transaction, error) {
	return _GodToken.contract.Transact(opts, "setName", _name)
}

// SetName is a paid mutator transaction binding the contract method 0xc47f0027.
//
// Solidity: function setName(_name string) returns()
func (_GodToken *GodTokenSession) SetName(_name string) (*types.Transaction, error) {
	return _GodToken.Contract.SetName(&_GodToken.TransactOpts, _name)
}

// SetName is a paid mutator transaction binding the contract method 0xc47f0027.
//
// Solidity: function setName(_name string) returns()
func (_GodToken *GodTokenTransactorSession) SetName(_name string) (*types.Transaction, error) {
	return _GodToken.Contract.SetName(&_GodToken.TransactOpts, _name)
}

// SetStakingRequirement is a paid mutator transaction binding the contract method 0x8328b610.
//
// Solidity: function setStakingRequirement(_amountOfTokens uint256) returns()
func (_GodToken *GodTokenTransactor) SetStakingRequirement(opts *bind.TransactOpts, _amountOfTokens *big.Int) (*types.Transaction, error) {
	return _GodToken.contract.Transact(opts, "setStakingRequirement", _amountOfTokens)
}

// SetStakingRequirement is a paid mutator transaction binding the contract method 0x8328b610.
//
// Solidity: function setStakingRequirement(_amountOfTokens uint256) returns()
func (_GodToken *GodTokenSession) SetStakingRequirement(_amountOfTokens *big.Int) (*types.Transaction, error) {
	return _GodToken.Contract.SetStakingRequirement(&_GodToken.TransactOpts, _amountOfTokens)
}

// SetStakingRequirement is a paid mutator transaction binding the contract method 0x8328b610.
//
// Solidity: function setStakingRequirement(_amountOfTokens uint256) returns()
func (_GodToken *GodTokenTransactorSession) SetStakingRequirement(_amountOfTokens *big.Int) (*types.Transaction, error) {
	return _GodToken.Contract.SetStakingRequirement(&_GodToken.TransactOpts, _amountOfTokens)
}

// SetSymbol is a paid mutator transaction binding the contract method 0xb84c8246.
//
// Solidity: function setSymbol(_symbol string) returns()
func (_GodToken *GodTokenTransactor) SetSymbol(opts *bind.TransactOpts, _symbol string) (*types.Transaction, error) {
	return _GodToken.contract.Transact(opts, "setSymbol", _symbol)
}

// SetSymbol is a paid mutator transaction binding the contract method 0xb84c8246.
//
// Solidity: function setSymbol(_symbol string) returns()
func (_GodToken *GodTokenSession) SetSymbol(_symbol string) (*types.Transaction, error) {
	return _GodToken.Contract.SetSymbol(&_GodToken.TransactOpts, _symbol)
}

// SetSymbol is a paid mutator transaction binding the contract method 0xb84c8246.
//
// Solidity: function setSymbol(_symbol string) returns()
func (_GodToken *GodTokenTransactorSession) SetSymbol(_symbol string) (*types.Transaction, error) {
	return _GodToken.Contract.SetSymbol(&_GodToken.TransactOpts, _symbol)
}

// TakeProjectBonus is a paid mutator transaction binding the contract method 0x629b9cb1.
//
// Solidity: function takeProjectBonus(to address, value uint256) returns()
func (_GodToken *GodTokenTransactor) TakeProjectBonus(opts *bind.TransactOpts, to common.Address, value *big.Int) (*types.Transaction, error) {
	return _GodToken.contract.Transact(opts, "takeProjectBonus", to, value)
}

// TakeProjectBonus is a paid mutator transaction binding the contract method 0x629b9cb1.
//
// Solidity: function takeProjectBonus(to address, value uint256) returns()
func (_GodToken *GodTokenSession) TakeProjectBonus(to common.Address, value *big.Int) (*types.Transaction, error) {
	return _GodToken.Contract.TakeProjectBonus(&_GodToken.TransactOpts, to, value)
}

// TakeProjectBonus is a paid mutator transaction binding the contract method 0x629b9cb1.
//
// Solidity: function takeProjectBonus(to address, value uint256) returns()
func (_GodToken *GodTokenTransactorSession) TakeProjectBonus(to common.Address, value *big.Int) (*types.Transaction, error) {
	return _GodToken.Contract.TakeProjectBonus(&_GodToken.TransactOpts, to, value)
}

// Transfer is a paid mutator transaction binding the contract method 0xa9059cbb.
//
// Solidity: function transfer(_toAddress address, _amountOfTokens uint256) returns(bool)
func (_GodToken *GodTokenTransactor) Transfer(opts *bind.TransactOpts, _toAddress common.Address, _amountOfTokens *big.Int) (*types.Transaction, error) {
	return _GodToken.contract.Transact(opts, "transfer", _toAddress, _amountOfTokens)
}

// Transfer is a paid mutator transaction binding the contract method 0xa9059cbb.
//
// Solidity: function transfer(_toAddress address, _amountOfTokens uint256) returns(bool)
func (_GodToken *GodTokenSession) Transfer(_toAddress common.Address, _amountOfTokens *big.Int) (*types.Transaction, error) {
	return _GodToken.Contract.Transfer(&_GodToken.TransactOpts, _toAddress, _amountOfTokens)
}

// Transfer is a paid mutator transaction binding the contract method 0xa9059cbb.
//
// Solidity: function transfer(_toAddress address, _amountOfTokens uint256) returns(bool)
func (_GodToken *GodTokenTransactorSession) Transfer(_toAddress common.Address, _amountOfTokens *big.Int) (*types.Transaction, error) {
	return _GodToken.Contract.Transfer(&_GodToken.TransactOpts, _toAddress, _amountOfTokens)
}

// TransferFrom is a paid mutator transaction binding the contract method 0x23b872dd.
//
// Solidity: function transferFrom(_from address, _toAddress address, _amountOfTokens uint256) returns(bool)
func (_GodToken *GodTokenTransactor) TransferFrom(opts *bind.TransactOpts, _from common.Address, _toAddress common.Address, _amountOfTokens *big.Int) (*types.Transaction, error) {
	return _GodToken.contract.Transact(opts, "transferFrom", _from, _toAddress, _amountOfTokens)
}

// TransferFrom is a paid mutator transaction binding the contract method 0x23b872dd.
//
// Solidity: function transferFrom(_from address, _toAddress address, _amountOfTokens uint256) returns(bool)
func (_GodToken *GodTokenSession) TransferFrom(_from common.Address, _toAddress common.Address, _amountOfTokens *big.Int) (*types.Transaction, error) {
	return _GodToken.Contract.TransferFrom(&_GodToken.TransactOpts, _from, _toAddress, _amountOfTokens)
}

// TransferFrom is a paid mutator transaction binding the contract method 0x23b872dd.
//
// Solidity: function transferFrom(_from address, _toAddress address, _amountOfTokens uint256) returns(bool)
func (_GodToken *GodTokenTransactorSession) TransferFrom(_from common.Address, _toAddress common.Address, _amountOfTokens *big.Int) (*types.Transaction, error) {
	return _GodToken.Contract.TransferFrom(&_GodToken.TransactOpts, _from, _toAddress, _amountOfTokens)
}

// TransferTo is a paid mutator transaction binding the contract method 0x19fb361f.
//
// Solidity: function transferTo(_from address, _to address, _amountOfTokens uint256, _data bytes) returns()
func (_GodToken *GodTokenTransactor) TransferTo(opts *bind.TransactOpts, _from common.Address, _to common.Address, _amountOfTokens *big.Int, _data []byte) (*types.Transaction, error) {
	return _GodToken.contract.Transact(opts, "transferTo", _from, _to, _amountOfTokens, _data)
}

// TransferTo is a paid mutator transaction binding the contract method 0x19fb361f.
//
// Solidity: function transferTo(_from address, _to address, _amountOfTokens uint256, _data bytes) returns()
func (_GodToken *GodTokenSession) TransferTo(_from common.Address, _to common.Address, _amountOfTokens *big.Int, _data []byte) (*types.Transaction, error) {
	return _GodToken.Contract.TransferTo(&_GodToken.TransactOpts, _from, _to, _amountOfTokens, _data)
}

// TransferTo is a paid mutator transaction binding the contract method 0x19fb361f.
//
// Solidity: function transferTo(_from address, _to address, _amountOfTokens uint256, _data bytes) returns()
func (_GodToken *GodTokenTransactorSession) TransferTo(_from common.Address, _to common.Address, _amountOfTokens *big.Int, _data []byte) (*types.Transaction, error) {
	return _GodToken.Contract.TransferTo(&_GodToken.TransactOpts, _from, _to, _amountOfTokens, _data)
}

// Withdraw is a paid mutator transaction binding the contract method 0x3ccfd60b.
//
// Solidity: function withdraw() returns()
func (_GodToken *GodTokenTransactor) Withdraw(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _GodToken.contract.Transact(opts, "withdraw")
}

// Withdraw is a paid mutator transaction binding the contract method 0x3ccfd60b.
//
// Solidity: function withdraw() returns()
func (_GodToken *GodTokenSession) Withdraw() (*types.Transaction, error) {
	return _GodToken.Contract.Withdraw(&_GodToken.TransactOpts)
}

// Withdraw is a paid mutator transaction binding the contract method 0x3ccfd60b.
//
// Solidity: function withdraw() returns()
func (_GodToken *GodTokenTransactorSession) Withdraw() (*types.Transaction, error) {
	return _GodToken.Contract.Withdraw(&_GodToken.TransactOpts)
}

// GodTokenApprovalIterator is returned from FilterApproval and is used to iterate over the raw logs and unpacked data for Approval events raised by the GodToken contract.
type GodTokenApprovalIterator struct {
	Event *GodTokenApproval // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *GodTokenApprovalIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(GodTokenApproval)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(GodTokenApproval)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *GodTokenApprovalIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *GodTokenApprovalIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// GodTokenApproval represents a Approval event raised by the GodToken contract.
type GodTokenApproval struct {
	Owner   common.Address
	Spender common.Address
	Value   *big.Int
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterApproval is a free log retrieval operation binding the contract event 0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925.
//
// Solidity: event Approval(_owner indexed address, _spender indexed address, _value uint256)
func (_GodToken *GodTokenFilterer) FilterApproval(opts *bind.FilterOpts, _owner []common.Address, _spender []common.Address) (*GodTokenApprovalIterator, error) {

	var _ownerRule []interface{}
	for _, _ownerItem := range _owner {
		_ownerRule = append(_ownerRule, _ownerItem)
	}
	var _spenderRule []interface{}
	for _, _spenderItem := range _spender {
		_spenderRule = append(_spenderRule, _spenderItem)
	}

	logs, sub, err := _GodToken.contract.FilterLogs(opts, "Approval", _ownerRule, _spenderRule)
	if err != nil {
		return nil, err
	}
	return &GodTokenApprovalIterator{contract: _GodToken.contract, event: "Approval", logs: logs, sub: sub}, nil
}

// WatchApproval is a free log subscription operation binding the contract event 0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925.
//
// Solidity: event Approval(_owner indexed address, _spender indexed address, _value uint256)
func (_GodToken *GodTokenFilterer) WatchApproval(opts *bind.WatchOpts, sink chan<- *GodTokenApproval, _owner []common.Address, _spender []common.Address) (event.Subscription, error) {

	var _ownerRule []interface{}
	for _, _ownerItem := range _owner {
		_ownerRule = append(_ownerRule, _ownerItem)
	}
	var _spenderRule []interface{}
	for _, _spenderItem := range _spender {
		_spenderRule = append(_spenderRule, _spenderItem)
	}

	logs, sub, err := _GodToken.contract.WatchLogs(opts, "Approval", _ownerRule, _spenderRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(GodTokenApproval)
				if err := _GodToken.contract.UnpackLog(event, "Approval", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// GodTokenTransferIterator is returned from FilterTransfer and is used to iterate over the raw logs and unpacked data for Transfer events raised by the GodToken contract.
type GodTokenTransferIterator struct {
	Event *GodTokenTransfer // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *GodTokenTransferIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(GodTokenTransfer)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(GodTokenTransfer)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *GodTokenTransferIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *GodTokenTransferIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// GodTokenTransfer represents a Transfer event raised by the GodToken contract.
type GodTokenTransfer struct {
	From   common.Address
	To     common.Address
	Tokens *big.Int
	Raw    types.Log // Blockchain specific contextual infos
}

// FilterTransfer is a free log retrieval operation binding the contract event 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef.
//
// Solidity: event Transfer(from indexed address, to indexed address, tokens uint256)
func (_GodToken *GodTokenFilterer) FilterTransfer(opts *bind.FilterOpts, from []common.Address, to []common.Address) (*GodTokenTransferIterator, error) {

	var fromRule []interface{}
	for _, fromItem := range from {
		fromRule = append(fromRule, fromItem)
	}
	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _GodToken.contract.FilterLogs(opts, "Transfer", fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return &GodTokenTransferIterator{contract: _GodToken.contract, event: "Transfer", logs: logs, sub: sub}, nil
}

// WatchTransfer is a free log subscription operation binding the contract event 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef.
//
// Solidity: event Transfer(from indexed address, to indexed address, tokens uint256)
func (_GodToken *GodTokenFilterer) WatchTransfer(opts *bind.WatchOpts, sink chan<- *GodTokenTransfer, from []common.Address, to []common.Address) (event.Subscription, error) {

	var fromRule []interface{}
	for _, fromItem := range from {
		fromRule = append(fromRule, fromItem)
	}
	var toRule []interface{}
	for _, toItem := range to {
		toRule = append(toRule, toItem)
	}

	logs, sub, err := _GodToken.contract.WatchLogs(opts, "Transfer", fromRule, toRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(GodTokenTransfer)
				if err := _GodToken.contract.UnpackLog(event, "Transfer", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// GodTokenOnInjectEtherFromIcoIterator is returned from FilterOnInjectEtherFromIco and is used to iterate over the raw logs and unpacked data for OnInjectEtherFromIco events raised by the GodToken contract.
type GodTokenOnInjectEtherFromIcoIterator struct {
	Event *GodTokenOnInjectEtherFromIco // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *GodTokenOnInjectEtherFromIcoIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(GodTokenOnInjectEtherFromIco)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(GodTokenOnInjectEtherFromIco)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *GodTokenOnInjectEtherFromIcoIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *GodTokenOnInjectEtherFromIcoIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// GodTokenOnInjectEtherFromIco represents a OnInjectEtherFromIco event raised by the GodToken contract.
type GodTokenOnInjectEtherFromIco struct {
	IncomingEthereum *big.Int
	Dividends        *big.Int
	ProfitPerShare   *big.Int
	Raw              types.Log // Blockchain specific contextual infos
}

// FilterOnInjectEtherFromIco is a free log retrieval operation binding the contract event 0xe25eb8bd212472a2c6a3fca01bdb1ce433b5fddb157d1c1ace49ac894989b76d.
//
// Solidity: event onInjectEtherFromIco(_incomingEthereum uint256, _dividends uint256, profitPerShare_ uint256)
func (_GodToken *GodTokenFilterer) FilterOnInjectEtherFromIco(opts *bind.FilterOpts) (*GodTokenOnInjectEtherFromIcoIterator, error) {

	logs, sub, err := _GodToken.contract.FilterLogs(opts, "onInjectEtherFromIco")
	if err != nil {
		return nil, err
	}
	return &GodTokenOnInjectEtherFromIcoIterator{contract: _GodToken.contract, event: "onInjectEtherFromIco", logs: logs, sub: sub}, nil
}

// WatchOnInjectEtherFromIco is a free log subscription operation binding the contract event 0xe25eb8bd212472a2c6a3fca01bdb1ce433b5fddb157d1c1ace49ac894989b76d.
//
// Solidity: event onInjectEtherFromIco(_incomingEthereum uint256, _dividends uint256, profitPerShare_ uint256)
func (_GodToken *GodTokenFilterer) WatchOnInjectEtherFromIco(opts *bind.WatchOpts, sink chan<- *GodTokenOnInjectEtherFromIco) (event.Subscription, error) {

	logs, sub, err := _GodToken.contract.WatchLogs(opts, "onInjectEtherFromIco")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(GodTokenOnInjectEtherFromIco)
				if err := _GodToken.contract.UnpackLog(event, "onInjectEtherFromIco", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// GodTokenOnInjectEtherToDividendIterator is returned from FilterOnInjectEtherToDividend and is used to iterate over the raw logs and unpacked data for OnInjectEtherToDividend events raised by the GodToken contract.
type GodTokenOnInjectEtherToDividendIterator struct {
	Event *GodTokenOnInjectEtherToDividend // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *GodTokenOnInjectEtherToDividendIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(GodTokenOnInjectEtherToDividend)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(GodTokenOnInjectEtherToDividend)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *GodTokenOnInjectEtherToDividendIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *GodTokenOnInjectEtherToDividendIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// GodTokenOnInjectEtherToDividend represents a OnInjectEtherToDividend event raised by the GodToken contract.
type GodTokenOnInjectEtherToDividend struct {
	Sender           common.Address
	IncomingEthereum *big.Int
	ProfitPerShare   *big.Int
	Raw              types.Log // Blockchain specific contextual infos
}

// FilterOnInjectEtherToDividend is a free log retrieval operation binding the contract event 0x0b7062c5e80b550628cf935ce6a62e41e9780c12387fc4aac59178dd7b4c7cbe.
//
// Solidity: event onInjectEtherToDividend(sender address, _incomingEthereum uint256, profitPerShare_ uint256)
func (_GodToken *GodTokenFilterer) FilterOnInjectEtherToDividend(opts *bind.FilterOpts) (*GodTokenOnInjectEtherToDividendIterator, error) {

	logs, sub, err := _GodToken.contract.FilterLogs(opts, "onInjectEtherToDividend")
	if err != nil {
		return nil, err
	}
	return &GodTokenOnInjectEtherToDividendIterator{contract: _GodToken.contract, event: "onInjectEtherToDividend", logs: logs, sub: sub}, nil
}

// WatchOnInjectEtherToDividend is a free log subscription operation binding the contract event 0x0b7062c5e80b550628cf935ce6a62e41e9780c12387fc4aac59178dd7b4c7cbe.
//
// Solidity: event onInjectEtherToDividend(sender address, _incomingEthereum uint256, profitPerShare_ uint256)
func (_GodToken *GodTokenFilterer) WatchOnInjectEtherToDividend(opts *bind.WatchOpts, sink chan<- *GodTokenOnInjectEtherToDividend) (event.Subscription, error) {

	logs, sub, err := _GodToken.contract.WatchLogs(opts, "onInjectEtherToDividend")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(GodTokenOnInjectEtherToDividend)
				if err := _GodToken.contract.UnpackLog(event, "onInjectEtherToDividend", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// GodTokenOnReinvestmentIterator is returned from FilterOnReinvestment and is used to iterate over the raw logs and unpacked data for OnReinvestment events raised by the GodToken contract.
type GodTokenOnReinvestmentIterator struct {
	Event *GodTokenOnReinvestment // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *GodTokenOnReinvestmentIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(GodTokenOnReinvestment)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(GodTokenOnReinvestment)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *GodTokenOnReinvestmentIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *GodTokenOnReinvestmentIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// GodTokenOnReinvestment represents a OnReinvestment event raised by the GodToken contract.
type GodTokenOnReinvestment struct {
	CustomerAddress    common.Address
	EthereumReinvested *big.Int
	TokensMinted       *big.Int
	Raw                types.Log // Blockchain specific contextual infos
}

// FilterOnReinvestment is a free log retrieval operation binding the contract event 0xbe339fc14b041c2b0e0f3dd2cd325d0c3668b78378001e53160eab3615326458.
//
// Solidity: event onReinvestment(customerAddress indexed address, ethereumReinvested uint256, tokensMinted uint256)
func (_GodToken *GodTokenFilterer) FilterOnReinvestment(opts *bind.FilterOpts, customerAddress []common.Address) (*GodTokenOnReinvestmentIterator, error) {

	var customerAddressRule []interface{}
	for _, customerAddressItem := range customerAddress {
		customerAddressRule = append(customerAddressRule, customerAddressItem)
	}

	logs, sub, err := _GodToken.contract.FilterLogs(opts, "onReinvestment", customerAddressRule)
	if err != nil {
		return nil, err
	}
	return &GodTokenOnReinvestmentIterator{contract: _GodToken.contract, event: "onReinvestment", logs: logs, sub: sub}, nil
}

// WatchOnReinvestment is a free log subscription operation binding the contract event 0xbe339fc14b041c2b0e0f3dd2cd325d0c3668b78378001e53160eab3615326458.
//
// Solidity: event onReinvestment(customerAddress indexed address, ethereumReinvested uint256, tokensMinted uint256)
func (_GodToken *GodTokenFilterer) WatchOnReinvestment(opts *bind.WatchOpts, sink chan<- *GodTokenOnReinvestment, customerAddress []common.Address) (event.Subscription, error) {

	var customerAddressRule []interface{}
	for _, customerAddressItem := range customerAddress {
		customerAddressRule = append(customerAddressRule, customerAddressItem)
	}

	logs, sub, err := _GodToken.contract.WatchLogs(opts, "onReinvestment", customerAddressRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(GodTokenOnReinvestment)
				if err := _GodToken.contract.UnpackLog(event, "onReinvestment", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// GodTokenOnTokenPurchaseIterator is returned from FilterOnTokenPurchase and is used to iterate over the raw logs and unpacked data for OnTokenPurchase events raised by the GodToken contract.
type GodTokenOnTokenPurchaseIterator struct {
	Event *GodTokenOnTokenPurchase // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *GodTokenOnTokenPurchaseIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(GodTokenOnTokenPurchase)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(GodTokenOnTokenPurchase)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *GodTokenOnTokenPurchaseIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *GodTokenOnTokenPurchaseIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// GodTokenOnTokenPurchase represents a OnTokenPurchase event raised by the GodToken contract.
type GodTokenOnTokenPurchase struct {
	CustomerAddress  common.Address
	IncomingEthereum *big.Int
	TokensMinted     *big.Int
	ReferredBy       common.Address
	Raw              types.Log // Blockchain specific contextual infos
}

// FilterOnTokenPurchase is a free log retrieval operation binding the contract event 0x022c0d992e4d873a3748436d960d5140c1f9721cf73f7ca5ec679d3d9f4fe2d5.
//
// Solidity: event onTokenPurchase(customerAddress indexed address, incomingEthereum uint256, tokensMinted uint256, referredBy indexed address)
func (_GodToken *GodTokenFilterer) FilterOnTokenPurchase(opts *bind.FilterOpts, customerAddress []common.Address, referredBy []common.Address) (*GodTokenOnTokenPurchaseIterator, error) {

	var customerAddressRule []interface{}
	for _, customerAddressItem := range customerAddress {
		customerAddressRule = append(customerAddressRule, customerAddressItem)
	}

	var referredByRule []interface{}
	for _, referredByItem := range referredBy {
		referredByRule = append(referredByRule, referredByItem)
	}

	logs, sub, err := _GodToken.contract.FilterLogs(opts, "onTokenPurchase", customerAddressRule, referredByRule)
	if err != nil {
		return nil, err
	}
	return &GodTokenOnTokenPurchaseIterator{contract: _GodToken.contract, event: "onTokenPurchase", logs: logs, sub: sub}, nil
}

// WatchOnTokenPurchase is a free log subscription operation binding the contract event 0x022c0d992e4d873a3748436d960d5140c1f9721cf73f7ca5ec679d3d9f4fe2d5.
//
// Solidity: event onTokenPurchase(customerAddress indexed address, incomingEthereum uint256, tokensMinted uint256, referredBy indexed address)
func (_GodToken *GodTokenFilterer) WatchOnTokenPurchase(opts *bind.WatchOpts, sink chan<- *GodTokenOnTokenPurchase, customerAddress []common.Address, referredBy []common.Address) (event.Subscription, error) {

	var customerAddressRule []interface{}
	for _, customerAddressItem := range customerAddress {
		customerAddressRule = append(customerAddressRule, customerAddressItem)
	}

	var referredByRule []interface{}
	for _, referredByItem := range referredBy {
		referredByRule = append(referredByRule, referredByItem)
	}

	logs, sub, err := _GodToken.contract.WatchLogs(opts, "onTokenPurchase", customerAddressRule, referredByRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(GodTokenOnTokenPurchase)
				if err := _GodToken.contract.UnpackLog(event, "onTokenPurchase", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// GodTokenOnTokenSellIterator is returned from FilterOnTokenSell and is used to iterate over the raw logs and unpacked data for OnTokenSell events raised by the GodToken contract.
type GodTokenOnTokenSellIterator struct {
	Event *GodTokenOnTokenSell // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *GodTokenOnTokenSellIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(GodTokenOnTokenSell)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(GodTokenOnTokenSell)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *GodTokenOnTokenSellIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *GodTokenOnTokenSellIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// GodTokenOnTokenSell represents a OnTokenSell event raised by the GodToken contract.
type GodTokenOnTokenSell struct {
	CustomerAddress common.Address
	TokensBurned    *big.Int
	EthereumEarned  *big.Int
	Raw             types.Log // Blockchain specific contextual infos
}

// FilterOnTokenSell is a free log retrieval operation binding the contract event 0xc4823739c5787d2ca17e404aa47d5569ae71dfb49cbf21b3f6152ed238a31139.
//
// Solidity: event onTokenSell(customerAddress indexed address, tokensBurned uint256, ethereumEarned uint256)
func (_GodToken *GodTokenFilterer) FilterOnTokenSell(opts *bind.FilterOpts, customerAddress []common.Address) (*GodTokenOnTokenSellIterator, error) {

	var customerAddressRule []interface{}
	for _, customerAddressItem := range customerAddress {
		customerAddressRule = append(customerAddressRule, customerAddressItem)
	}

	logs, sub, err := _GodToken.contract.FilterLogs(opts, "onTokenSell", customerAddressRule)
	if err != nil {
		return nil, err
	}
	return &GodTokenOnTokenSellIterator{contract: _GodToken.contract, event: "onTokenSell", logs: logs, sub: sub}, nil
}

// WatchOnTokenSell is a free log subscription operation binding the contract event 0xc4823739c5787d2ca17e404aa47d5569ae71dfb49cbf21b3f6152ed238a31139.
//
// Solidity: event onTokenSell(customerAddress indexed address, tokensBurned uint256, ethereumEarned uint256)
func (_GodToken *GodTokenFilterer) WatchOnTokenSell(opts *bind.WatchOpts, sink chan<- *GodTokenOnTokenSell, customerAddress []common.Address) (event.Subscription, error) {

	var customerAddressRule []interface{}
	for _, customerAddressItem := range customerAddress {
		customerAddressRule = append(customerAddressRule, customerAddressItem)
	}

	logs, sub, err := _GodToken.contract.WatchLogs(opts, "onTokenSell", customerAddressRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(GodTokenOnTokenSell)
				if err := _GodToken.contract.UnpackLog(event, "onTokenSell", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// GodTokenOnWithdrawIterator is returned from FilterOnWithdraw and is used to iterate over the raw logs and unpacked data for OnWithdraw events raised by the GodToken contract.
type GodTokenOnWithdrawIterator struct {
	Event *GodTokenOnWithdraw // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *GodTokenOnWithdrawIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(GodTokenOnWithdraw)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(GodTokenOnWithdraw)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *GodTokenOnWithdrawIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *GodTokenOnWithdrawIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// GodTokenOnWithdraw represents a OnWithdraw event raised by the GodToken contract.
type GodTokenOnWithdraw struct {
	CustomerAddress   common.Address
	EthereumWithdrawn *big.Int
	Raw               types.Log // Blockchain specific contextual infos
}

// FilterOnWithdraw is a free log retrieval operation binding the contract event 0xccad973dcd043c7d680389db4378bd6b9775db7124092e9e0422c9e46d7985dc.
//
// Solidity: event onWithdraw(customerAddress indexed address, ethereumWithdrawn uint256)
func (_GodToken *GodTokenFilterer) FilterOnWithdraw(opts *bind.FilterOpts, customerAddress []common.Address) (*GodTokenOnWithdrawIterator, error) {

	var customerAddressRule []interface{}
	for _, customerAddressItem := range customerAddress {
		customerAddressRule = append(customerAddressRule, customerAddressItem)
	}

	logs, sub, err := _GodToken.contract.FilterLogs(opts, "onWithdraw", customerAddressRule)
	if err != nil {
		return nil, err
	}
	return &GodTokenOnWithdrawIterator{contract: _GodToken.contract, event: "onWithdraw", logs: logs, sub: sub}, nil
}

// WatchOnWithdraw is a free log subscription operation binding the contract event 0xccad973dcd043c7d680389db4378bd6b9775db7124092e9e0422c9e46d7985dc.
//
// Solidity: event onWithdraw(customerAddress indexed address, ethereumWithdrawn uint256)
func (_GodToken *GodTokenFilterer) WatchOnWithdraw(opts *bind.WatchOpts, sink chan<- *GodTokenOnWithdraw, customerAddress []common.Address) (event.Subscription, error) {

	var customerAddressRule []interface{}
	for _, customerAddressItem := range customerAddress {
		customerAddressRule = append(customerAddressRule, customerAddressItem)
	}

	logs, sub, err := _GodToken.contract.WatchLogs(opts, "onWithdraw", customerAddressRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(GodTokenOnWithdraw)
				if err := _GodToken.contract.UnpackLog(event, "onWithdraw", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}
