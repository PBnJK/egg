// Package egg/z80 implements a Zilog Z80 machine for EGG.
//
// Reference:
// - https://www.zilog.com/docs/z80/um0080.pdf
package z80

import (
	"github.com/gboncoffee/egg/assembler"
	"github.com/gboncoffee/egg/machine"
)

const (
	HI = 0 // Access HI register
	LO = 1 // Access LO register
)

// The Z80 struct implements the Machine interface
type Z80 struct {
	// The Z80 registers.
	//
	// A lot of them are "linked", that is, two 8-bit registers that can be used
	// as a single 16-bit register. So A and F can be accessed as AF, and so on.
	//
	// These are implemented as uint16 variables, and the HI/LO parts are accessed
	// using bitwise math.
	registers struct {
		AF uint16 // Accumulator/Flag
		BC uint16
		DE uint16
		HL uint16
		PC uint16 // Program counter
		SP uint16 // Stack pointer
	}

	ix uint16
	iy uint16
}

func (m *Z80) LoadProgram([]uint8) error {
	return nil
}

func (m *Z80) NextInstruction() (*machine.Call, error) {
	return nil, nil
}

func (m *Z80) GetMemory(uint64) (uint8, error) {
	return 0, nil
}

func (m *Z80) SetMemory(uint64, uint8) error {
	return nil
}

func (m *Z80) GetMemoryChunk(uint64, uint64) ([]uint8, error) {
	return nil, nil
}

func (m *Z80) SetMemoryChunk(uint64, []uint8) error {
	return nil
}

func (m *Z80) GetRegister(uint64) (uint64, error) {
	return 0, nil
}

func (m *Z80) SetRegister(uint64, uint64) error {
	return nil
}

func (m *Z80) GetRegisterNumber(string) (uint64, error) {
	return 0, nil
}

// FIXME: Write this function
func assemble(t []assembler.ResolvedToken) ([]uint8, error) {
	code := make([]uint8, 8)
	return code, nil
}

func (m *Z80) Assemble(file string) ([]uint8, []assembler.DebuggerToken, error) {
	tokens := []assembler.Token{}
	err := assembler.Tokenize(file, &tokens)
	if err != nil {
		return nil, nil, err
	}

	resolvedTokens, debuggerTokens, err := assembler.ResolveTokens(tokens, func(i *assembler.Instruction) error {
		i.Size = 1
		return nil
	}, nil)
	if err != nil {
		return nil, nil, err
	}

	code, err := assemble(resolvedTokens)
	if err != nil {
		return nil, nil, err
	}

	return code, debuggerTokens, nil
}

func (m *Z80) GetCurrentInstructionAddress() uint64 {
	return uint64(m.registers.PC)
}

func (m *Z80) ArchitectureInfo() machine.ArchitectureInfo {
	return machine.ArchitectureInfo{
		// TODO: Is brand name OK?
		Name: "Zilog Z80",
		// Words are actually 8-bit. We use 16 as that is the address size
		WordWidth: 16,
		RegistersNames: []string{
			"A", "F", "AF",
			"B", "C", "BC",
			"D", "E", "DE",
			"H", "L", "HL",
			"PC",
		},
	}
}
