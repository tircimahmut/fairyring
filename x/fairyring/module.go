package fairyring

import (
	"context"
	"encoding/json"
	"fmt"
	// this line is used by starport scaffolding # 1

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/spf13/cobra"

	abci "github.com/tendermint/tendermint/abci/types"

	"fairyring/x/fairyring/client/cli"
	"fairyring/x/fairyring/keeper"
	"fairyring/x/fairyring/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
)

var (
	_ module.AppModule      = AppModule{}
	_ module.AppModuleBasic = AppModuleBasic{}
)

// ----------------------------------------------------------------------------
// AppModuleBasic
// ----------------------------------------------------------------------------

// AppModuleBasic implements the AppModuleBasic interface that defines the independent methods a Cosmos SDK module needs to implement.
type AppModuleBasic struct {
	cdc codec.BinaryCodec
}

func NewAppModuleBasic(cdc codec.BinaryCodec) AppModuleBasic {
	return AppModuleBasic{cdc: cdc}
}

// Name returns the name of the module as a string
func (AppModuleBasic) Name() string {
	return types.ModuleName
}

// RegisterLegacyAminoCodec registers the amino codec for the module, which is used to marshal and unmarshal structs to/from []byte in order to persist them in the module's KVStore
func (AppModuleBasic) RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	types.RegisterCodec(cdc)
}

// RegisterInterfaces registers a module's interface types and their concrete implementations as proto.Message
func (a AppModuleBasic) RegisterInterfaces(reg cdctypes.InterfaceRegistry) {
	types.RegisterInterfaces(reg)
}

// DefaultGenesis returns a default GenesisState for the module, marshalled to json.RawMessage. The default GenesisState need to be defined by the module developer and is primarily used for testing
func (AppModuleBasic) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
	return cdc.MustMarshalJSON(types.DefaultGenesis())
}

// ValidateGenesis used to validate the GenesisState, given in its json.RawMessage form
func (AppModuleBasic) ValidateGenesis(cdc codec.JSONCodec, config client.TxEncodingConfig, bz json.RawMessage) error {
	var genState types.GenesisState
	if err := cdc.UnmarshalJSON(bz, &genState); err != nil {
		return fmt.Errorf("failed to unmarshal %s genesis state: %w", types.ModuleName, err)
	}
	return genState.Validate()
}

// RegisterGRPCGatewayRoutes registers the gRPC Gateway routes for the module
func (AppModuleBasic) RegisterGRPCGatewayRoutes(clientCtx client.Context, mux *runtime.ServeMux) {
	types.RegisterQueryHandlerClient(context.Background(), mux, types.NewQueryClient(clientCtx))
}

// GetTxCmd returns the root Tx command for the module. The subcommands of this root command are used by end-users to generate new transactions containing messages defined in the module
func (a AppModuleBasic) GetTxCmd() *cobra.Command {
	return cli.GetTxCmd()
}

// GetQueryCmd returns the root query command for the module. The subcommands of this root command are used by end-users to generate new queries to the subset of the state defined by the module
func (AppModuleBasic) GetQueryCmd() *cobra.Command {
	return cli.GetQueryCmd(types.StoreKey)
}

// ----------------------------------------------------------------------------
// AppModule
// ----------------------------------------------------------------------------

// AppModule implements the AppModule interface that defines the inter-dependent methods that modules need to implement
type AppModule struct {
	AppModuleBasic

	keeper        keeper.Keeper
	accountKeeper types.AccountKeeper
	bankKeeper    types.BankKeeper
}

func NewAppModule(
	cdc codec.Codec,
	keeper keeper.Keeper,
	accountKeeper types.AccountKeeper,
	bankKeeper types.BankKeeper,
) AppModule {
	return AppModule{
		AppModuleBasic: NewAppModuleBasic(cdc),
		keeper:         keeper,
		accountKeeper:  accountKeeper,
		bankKeeper:     bankKeeper,
	}
}

// Deprecated: use RegisterServices
func (am AppModule) Route() sdk.Route { return sdk.Route{} }

// Deprecated: use RegisterServices
func (AppModule) QuerierRoute() string { return types.RouterKey }

// Deprecated: use RegisterServices
func (am AppModule) LegacyQuerierHandler(_ *codec.LegacyAmino) sdk.Querier {
	return nil
}

// RegisterServices registers a gRPC query service to respond to the module-specific gRPC queries
func (am AppModule) RegisterServices(cfg module.Configurator) {
	types.RegisterMsgServer(cfg.MsgServer(), keeper.NewMsgServerImpl(am.keeper))
	types.RegisterQueryServer(cfg.QueryServer(), am.keeper)
}

// RegisterInvariants registers the invariants of the module. If an invariant deviates from its predicted value, the InvariantRegistry triggers appropriate logic (most often the chain will be halted)
func (am AppModule) RegisterInvariants(_ sdk.InvariantRegistry) {}

// InitGenesis performs the module's genesis initialization. It returns no validator updates.
func (am AppModule) InitGenesis(ctx sdk.Context, cdc codec.JSONCodec, gs json.RawMessage) []abci.ValidatorUpdate {
	var genState types.GenesisState
	// Initialize global index to index in genesis state
	cdc.MustUnmarshalJSON(gs, &genState)

	InitGenesis(ctx, am.keeper, genState)

	return []abci.ValidatorUpdate{}
}

// ExportGenesis returns the module's exported genesis state as raw JSON bytes.
func (am AppModule) ExportGenesis(ctx sdk.Context, cdc codec.JSONCodec) json.RawMessage {
	genState := ExportGenesis(ctx, am.keeper)
	return cdc.MustMarshalJSON(genState)
}

// ConsensusVersion is a sequence number for state-breaking change of the module. It should be incremented on each consensus-breaking change introduced by the module. To avoid wrong/empty versions, the initial version should be set to 1
func (AppModule) ConsensusVersion() uint64 { return 1 }

// BeginBlock contains the logic that is automatically triggered at the beginning of each block
func (am AppModule) BeginBlock(ctx sdk.Context, _ abci.RequestBeginBlock) {
	validators := am.keeper.StakingKeeper().GetAllValidators(ctx)
	for _, eachValidator := range validators {
		// if the validator is not bonded
		// ? add check bonded amount ?
		if !eachValidator.IsBonded() {
			valAddr, _ := sdk.ValAddressFromBech32(eachValidator.OperatorAddress)
			valAccAddr := sdk.AccAddress(valAddr)
			// Remove it from validator set to prevent it submitting keyshares
			am.keeper.RemoveValidatorSet(ctx, valAccAddr.String())

			consAddr, _ := eachValidator.GetConsAddr()
			// Slash the validator
			am.keeper.StakingKeeper().Slash(ctx, consAddr, ctx.BlockHeight()+1, 10, sdk.NewDec(10))
		}
	}
	//validatorList := am.keeper.GetAllValidatorSet(ctx)
	//
	//suite := bls.NewBLS12381Suite()
	//
	//var listOfShares []distIBE.ExtractedKey
	//var listOfCommitment []distIBE.Commitment
	//
	//for _, eachValidator := range validatorList {
	//	eachKeyShare, found := am.keeper.GetKeyShare(ctx, eachValidator.Validator, uint64(ctx.BlockHeight()))
	//	if !found {
	//		am.keeper.Logger(ctx).Info(
	//			fmt.Sprintf(
	//				"Can not find key share from validator: %s for height: %d",
	//				eachValidator.Validator,
	//				ctx.BlockHeight(),
	//			),
	//		)
	//		continue
	//	}
	//
	//	byteKey, err := hex.DecodeString(eachKeyShare.KeyShare)
	//	if err != nil {
	//		am.keeper.Logger(ctx).Error(fmt.Sprintf("Error in decoding hex key: %s", err.Error()))
	//		continue
	//	}
	//
	//	kp := suite.G2().Point()
	//	err = kp.UnmarshalBinary(byteKey)
	//	if err != nil {
	//		am.keeper.Logger(ctx).Error(fmt.Sprintf("Error in unmarshal point: %s", err.Error()))
	//		continue
	//	}
	//
	//	am.keeper.Logger(ctx).Info(eachKeyShare.Commitment)
	//	byteCommitment, err := hex.DecodeString(eachKeyShare.Commitment)
	//	if err != nil {
	//		am.keeper.Logger(ctx).Error(fmt.Sprintf("Error in decoding hex commitment: %s", err.Error()))
	//		continue
	//	}
	//
	//	commitmentKp := suite.G1().Point()
	//	err = commitmentKp.UnmarshalBinary(byteCommitment)
	//	if err != nil {
	//		am.keeper.Logger(ctx).Error(fmt.Sprintf("Error in unmarshal commitment point: %s", err.Error()))
	//		continue
	//	}
	//
	//	listOfShares = append(
	//		listOfShares,
	//		distIBE.ExtractedKey{
	//			Sk:    kp,
	//			Index: uint32(eachKeyShare.KeyShareIndex),
	//		},
	//	)
	//	listOfCommitment = append(
	//		listOfCommitment,
	//		distIBE.Commitment{
	//			Sp:    commitmentKp,
	//			Index: uint32(eachKeyShare.KeyShareIndex),
	//		},
	//	)
	//}
	//
	//if len(listOfCommitment) > 0 && len(listOfShares) > 0 {
	//	SK, _ := distIBE.AggregateSK(suite, listOfShares, listOfCommitment, []byte(types.IBEId))
	//	am.keeper.Logger(ctx).Info(fmt.Sprintf("Aggregated Decryption Key: %s", SK.String()))
	//}
}

// EndBlock contains the logic that is automatically triggered at the end of each block
func (am AppModule) EndBlock(_ sdk.Context, _ abci.RequestEndBlock) []abci.ValidatorUpdate {
	return []abci.ValidatorUpdate{}
}
