# Swarm ENS interface

## Usage
以太坊名称服务的完整文档[可以在EIP 137中找到]（https://github.com/ethereum/EIPs/issues/137）。
这个包提供了一个简单的绑定，简化了任意UTF8域名到swarm内容哈希的注册。

##开发

contract子目录中的SOL文件实现了ENS根注册表，这很简单
根名称空间的先进先服务注册商和简单的解析器合同;
它们用于测试，可用于为您自己的目的部署这些合同。

可以在[github.com/arachnid/ens/](https://github.com/arachnid/ens/）找到可靠性源代码。

ENS合约的go绑定是通过go生成器使用`abigen`生成的：
Full documentation for the Ethereum Name Service [can be found as EIP 137](https://github.com/ethereum/EIPs/issues/137).
This package offers a simple binding that streamlines the registration of arbitrary UTF8 domain names to swarm content hashes.

## Development

The SOL file in contract subdirectory implements the ENS root registry, a simple
first-in, first-served registrar for the root namespace, and a simple resolver contract;
they're used in tests, and can be used to deploy these contracts for your own purposes.

The solidity source code can be found at [github.com/arachnid/ens/](https://github.com/arachnid/ens/).

The go bindings for ENS contracts are generated using `abigen` via the go generator:

```shell
go generate ./contracts/ens
```
