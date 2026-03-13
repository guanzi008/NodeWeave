module nodeweave/clients/windows-agent

go 1.26.1

require (
	nodeweave/packages/contracts/go v0.0.0
	nodeweave/packages/runtime/go v0.0.0
)

replace nodeweave/packages/contracts/go => ../../packages/contracts/go
replace nodeweave/packages/runtime/go => ../../packages/runtime/go
