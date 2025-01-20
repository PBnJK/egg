// Package egg/z80 implements a Zilog Z80 machine for EGG.
//
// The Z80 has a lot of registers. We've settled on this numbering system:
// - A:   0
// - F:   1
// - AF:  2
// - B:   3
// - C:   4
// - BC:  5
// - D:   6
// - E:   7
// - DE:  8
// - H:   9
// - L:   10
// - HL:  11
// - A':  12
// - F':  13
// - ...
// - HL': 23
// - I:   24
// - R:   25
// - IX:  26
// - IY:  27
// - SP:  28
// - PC:  29
//
// Following the example of the MOS 6502, programs are loaded at the TEXTPAGE
// instead of at 0, as is the actual behaviour of the Z80. Code is written, as
// such, to 0x8000 onwards.
//
// The HALT instruction is used for system calls, as the Z80 lacks a dedicated
// instruction, similarly to the MOS 6502 emulator.
//
// Reference:
// - https://www.zilog.com/docs/z80/um0080.pdf
package z80

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/gboncoffee/egg/assembler"
	"github.com/gboncoffee/egg/machine"
)

const (
	HI = 0 // Access HI register
	LO = 1 // Access LO register

	TEXTPAGE = 0x8000

	// Set if the last subtraction was negative, or if there was an overflow
	CARRY_FLAG = 0b00000001
	// Set if the last instruction used on the Accumulator was a subtraction
	ADDSUB_FLAG = 0b00000010
	// Either holds the result of parity or overflow, depending on the last
	// instruction
	OVERFLOW_FLAG = 0b00000100
	// Determines if the least-significant nibble had a carry
	HALF_CARRY_FLAG = 0b00010000
	// Determines whether the Accumulator is 0
	ZERO_FLAG = 0b01000000
	// Set if the Accumulator is positive. Assumes that the Accumulator is signed
	SIGNED_FLAG = 0b10000000
)

type Register struct {
	Low  uint8
	High uint8
}

func (r Register) Get16() uint64 {
	return (uint64(r.High) << 8) | uint64(r.Low)
}

func (r *Register) Set16(content uint16) {
	r.High = uint8(content >> 8)
	r.Low = uint8(content & 0xFF)
}

// The Z80 struct implements the Machine interface
type Z80 struct {
	// The Z80 registers.
	//
	// A lot of them are "linked", that is, two 8-bit registers that can be used
	// as a single 16-bit register. So A and F can be accessed as AF, and so on.
	//
	// The 'p' suffix indicates the alternate set of registers. The Z80 allows you
	// to switch between these two sets at will.
	registers struct {
		AF Register // Accumulator/Flag register
		BC Register
		DE Register
		HL Register

		AFp Register
		BCp Register
		DEp Register
		HLp Register

		I uint8 // Interrupt vector
		R uint8 // Memory refresh

		IX uint16 // Index X
		IY uint16 // Index Y
		SP uint16 // Stack pointer
		PC uint16 // Program counter
	}

	mem [math.MaxUint16 + 1]uint8
}

func (m *Z80) LoadProgram(program []uint8) error {
	m.registers.PC = 0
	return m.SetMemoryChunk(TEXTPAGE, program)
}

func (m *Z80) next8() uint8 {
	byte, _ := m.GetMemory(m.GetCurrentInstructionAddress())
	m.registers.PC++
	return byte
}

func (m *Z80) next16() uint16 {
	values, _ := m.GetMemoryChunk(m.GetCurrentInstructionAddress(), 2)
	m.registers.PC += 2
	return binary.BigEndian.Uint16(values)
}

// Handles instruction in the format
// DD [OPCODE] ...
func (m *Z80) handleDDInstruction() (*machine.Call, error) {
	opcode := m.next8()

	if opcode&0b111 == 0b110 {
		switch (opcode & 0b11000000) >> 6 {
		case 0b00:
			return nil, nil

		// LD r, (IX+d)
		case 0b01:
			r := m.numToRegister((opcode & 0b00111000) >> 3)
			// TODO: there's gotta be a nicer way to do this
			//       (there is, I'm just putting it off for Later(tm))
			*r, _ = m.GetMemory(uint64(int8(m.registers.IX) + int8(m.next8())))
			return nil, nil
		}
	}

	switch opcode {
	case 0x21:
		nn := m.next16()
		m.registers.IX = nn
	}

	return nil, nil
}

// Handles instruction in the format
// ED [OPCODE] ...
func (m *Z80) handleEDInstruction() (*machine.Call, error) {
	opcode := m.next8()

	switch opcode {
	case 0b01001011:
		nn := m.next16()
		m.registers.IX = nn
	}

	return nil, nil
}

// Handles instruction in the format
// FD [OPCODE] ...
func (m *Z80) handleFDInstruction() (*machine.Call, error) {
	opcode := m.next8()

	if opcode&0b111 == 0b110 {
		switch (opcode & 0b11000000) >> 6 {
		case 0b00:
			return nil, nil

		// LD r, (IY+d)
		case 0b01:
			r := m.numToRegister((opcode & 0b00111000) >> 3)
			// TODO: make nicer
			*r, _ = m.GetMemory(uint64(int8(m.registers.IY) + int8(m.next8())))
			return nil, nil
		}
	}

	switch opcode {
	case 0x21:
		nn := m.next16()
		m.registers.IY = nn
	}

	return nil, nil
}

func (m *Z80) numToRegister(num uint8) *uint8 {
	switch num {
	case 0b000:
		return &m.registers.BC.Low
	case 0b001:
		return &m.registers.BC.High
	case 0b010:
		return &m.registers.DE.Low
	case 0b011:
		return &m.registers.DE.High
	case 0b100:
		return &m.registers.HL.Low
	case 0b101:
		return &m.registers.HL.High
	case 0b111:
		return &m.registers.AF.Low
	}

	return nil
}

func (m *Z80) NextInstruction() (*machine.Call, error) {
	opcode := m.next8()

	// TODO: Remove all of this stuff. This is ugly, buggy... just plain *bad*!
	if (opcode&0b11000000)>>6 == 0b01 {
		// LD (HL), r
		if opcode&0b111000 == 0b110 {
			r := m.numToRegister(opcode & 0b111)
			m.SetMemory(uint64(m.registers.HL.Get16()), *r)
			return nil, nil
		}

		// LD r, r'
		r := m.numToRegister((opcode & 0b00111000) >> 3)
		*r = *m.numToRegister(opcode & 0b111)
		return nil, nil
	}

	if opcode&0b111 == 0b110 {
		switch (opcode & 0b11000000) >> 6 {
		// LD r, n
		case 0b00:
			r := m.numToRegister((opcode & 0b00111000) >> 3)
			*r = m.next8()
			return nil, nil
		// LD r, (HL)
		case 0b01:
			r := m.numToRegister((opcode & 0b00111000) >> 3)
			*r, _ = m.GetMemory(m.registers.HL.Get16())
			return nil, nil
		}
	}

	switch opcode {
	// NOP
	case 0:
		return nil, nil
	// LD BC, nn
	case 0b00000001:
		nn := m.next16()
		m.registers.BC.Set16(nn)
	// LD DE, nn
	case 0b00010001:
		nn := m.next16()
		m.registers.DE.Set16(nn)
	// LD HL, nn
	case 0b00100001:
		nn := m.next16()
		m.registers.HL.Set16(nn)
	// LD SP, nn
	case 0b00110001:
		nn := m.next16()
		m.registers.SP = nn
	// LD HL, (nn)
	case 0x2A:
		nn, _ := m.GetMemoryChunk(m.GetCurrentInstructionAddress(), 2)
		m.registers.PC += 2
		m.SetMemoryChunk(m.registers.HL.Get16(), nn)
	// 0xDD instruction subset
	case 0xDD:
		return m.handleDDInstruction()
	// 0xED instruction subset
	case 0xED:
		return m.handleEDInstruction()
	// 0xFD instruction subset
	case 0xFD:
		return m.handleFDInstruction()
	}

	return nil, nil
}

func (m *Z80) GetMemory(addr uint64) (uint8, error) {
	if addr > math.MaxUint16 {
		return 0, fmt.Errorf(
			machine.InterCtx.Get(
				"value %v is bigger than maximum 16 bit address %v",
			), addr, math.MaxUint16,
		)
	}

	return m.mem[addr], nil
}

func (m *Z80) SetMemory(addr uint64, content uint8) error {
	if addr > math.MaxUint16 {
		return fmt.Errorf(
			machine.InterCtx.Get(
				"value %v is bigger than maximum 16 bit address %v",
			), addr, math.MaxUint16,
		)
	}

	m.mem[addr] = content
	return nil
}

func (m *Z80) GetMemoryChunk(addr uint64, size uint64) ([]uint8, error) {
	end := addr + (size - 1)
	if end > math.MaxUint16 {
		return nil, fmt.Errorf(
			machine.InterCtx.Get(
				"end address %v is bigger than maximum 16 bit address %v",
			), addr, math.MaxUint16,
		)
	}

	return m.mem[addr:(end + 1)], nil
}

func (m *Z80) SetMemoryChunk(addr uint64, content []uint8) error {
	end := addr + uint64(len(content))
	if end > math.MaxUint16 {
		return fmt.Errorf(
			machine.InterCtx.Get(
				"end address %v is bigger than maximum 16 bit address %v",
			), addr, math.MaxUint16,
		)
	}

	for _, b := range content {
		m.mem[addr] = b
		addr++
	}

	return nil
}

func (m *Z80) GetRegister(r uint64) (uint64, error) {
	switch r {
	case 0:
		return uint64(m.registers.AF.High), nil
	case 1:
		return uint64(m.registers.AF.Low), nil
	case 2:
		return m.registers.AF.Get16(), nil
	case 3:
		return uint64(m.registers.BC.High), nil
	case 4:
		return uint64(m.registers.BC.Low), nil
	case 5:
		return m.registers.BC.Get16(), nil
	case 6:
		return uint64(m.registers.DE.High), nil
	case 7:
		return uint64(m.registers.DE.Low), nil
	case 8:
		return m.registers.DE.Get16(), nil
	case 9:
		return uint64(m.registers.HL.High), nil
	case 10:
		return uint64(m.registers.HL.Low), nil
	case 11:
		return m.registers.HL.Get16(), nil
	case 12:
		return uint64(m.registers.AFp.High), nil
	case 13:
		return uint64(m.registers.AFp.Low), nil
	case 14:
		return m.registers.AFp.Get16(), nil
	case 15:
		return uint64(m.registers.BCp.High), nil
	case 16:
		return uint64(m.registers.BCp.Low), nil
	case 17:
		return m.registers.BCp.Get16(), nil
	case 18:
		return uint64(m.registers.DEp.High), nil
	case 19:
		return uint64(m.registers.DEp.Low), nil
	case 20:
		return m.registers.DEp.Get16(), nil
	case 21:
		return uint64(m.registers.HLp.High), nil
	case 22:
		return uint64(m.registers.HLp.Low), nil
	case 23:
		return m.registers.HLp.Get16(), nil
	case 24:
		return uint64(m.registers.I), nil
	case 25:
		return uint64(m.registers.R), nil
	case 26:
		return uint64(m.registers.IX), nil
	case 27:
		return uint64(m.registers.IY), nil
	case 28:
		return uint64(m.registers.SP), nil
	case 29:
		return uint64(m.registers.PC), nil
	default:
		return 0, fmt.Errorf(machine.InterCtx.Get("no such register %v"), r)
	}
}

func (m *Z80) SetRegister(r uint64, content uint64) error {
	switch r {
	case 0:
		m.registers.AF.High = uint8(content)
	case 1:
		m.registers.AF.Low = uint8(content)
	case 2:
		m.registers.AF.Set16(uint16(content))
	case 3:
		m.registers.BC.High = uint8(content)
	case 4:
		m.registers.BC.Low = uint8(content)
	case 5:
		m.registers.BC.Set16(uint16(content))
	case 6:
		m.registers.DE.High = uint8(content)
	case 7:
		m.registers.DE.Low = uint8(content)
	case 8:
		m.registers.DE.Set16(uint16(content))
	case 9:
		m.registers.HL.High = uint8(content)
	case 10:
		m.registers.HL.Low = uint8(content)
	case 11:
		m.registers.HL.Set16(uint16(content))
	case 12:
		m.registers.AFp.High = uint8(content)
	case 13:
		m.registers.AFp.Low = uint8(content)
	case 14:
		m.registers.AFp.Set16(uint16(content))
	case 15:
		m.registers.BCp.High = uint8(content)
	case 16:
		m.registers.BCp.Low = uint8(content)
	case 17:
		m.registers.BCp.Set16(uint16(content))
	case 18:
		m.registers.DEp.High = uint8(content)
	case 19:
		m.registers.DEp.Low = uint8(content)
	case 20:
		m.registers.DEp.Set16(uint16(content))
	case 21:
		m.registers.HLp.High = uint8(content)
	case 22:
		m.registers.HLp.Low = uint8(content)
	case 23:
		m.registers.HLp.Set16(uint16(content))
	case 24:
		m.registers.I = uint8(content)
	case 25:
		m.registers.R = uint8(content)
	case 26:
		m.registers.IX = uint16(content)
	case 27:
		m.registers.IY = uint16(content)
	case 28:
		m.registers.SP = uint16(content)
	case 29:
		m.registers.PC = uint16(content)
	}

	return fmt.Errorf(machine.InterCtx.Get("no such register %v"), r)
}

func (m *Z80) GetRegisterNumber(r string) (uint64, error) {
	switch r {
	case "A":
		return 0, nil
	case "F":
		return 1, nil
	case "AF":
		return 2, nil
	case "B":
		return 3, nil
	case "C":
		return 4, nil
	case "BC":
		return 5, nil
	case "D":
		return 6, nil
	case "E":
		return 7, nil
	case "DE":
		return 8, nil
	case "H":
		return 9, nil
	case "L":
		return 10, nil
	case "HL":
		return 11, nil
	case "A'":
		return 12, nil
	case "F'":
		return 13, nil
	case "AF'":
		return 14, nil
	case "B'":
		return 15, nil
	case "C'":
		return 16, nil
	case "BC'":
		return 17, nil
	case "D'":
		return 18, nil
	case "E'":
		return 19, nil
	case "DE'":
		return 20, nil
	case "H'":
		return 21, nil
	case "L'":
		return 22, nil
	case "HL'":
		return 23, nil
	case "I":
		return 24, nil
	case "R":
		return 25, nil
	case "IX":
		return 26, nil
	case "IY":
		return 27, nil
	case "SP":
		return 28, nil
	case "PC":
		return 29, nil
	default:
		return 0, fmt.Errorf(machine.InterCtx.Get("no such register: %v"), r)
	}
}

func wrongArgsError(instruction string) error {
	return fmt.Errorf(machine.InterCtx.Get("wrong number of arguments for instruction '%s', expected 2 arguments"), instruction)
}

func assembleLd(t assembler.ResolvedToken) (uint32, int, error) {
	if len(t.Args) != 2 {
		return 0, 0, wrongArgsError(string(t.Value))
	}

	return 0, 2, nil
}

func assembleInstruction(code []uint8, t assembler.ResolvedToken) (int, error) {
	bin := uint32(0)
	size := 0
	var err error = nil

	switch string(t.Value) {
	case "ld":
		bin, size, err = assembleLd(t)
	}

	if err != nil {
		return 0, err
	}

	for i := range size {
		code = append(code, uint8((bin>>i)&0xFF))
	}

	return size, nil
}

func assemble(t []assembler.ResolvedToken) ([]uint8, error) {
	code := make([]uint8, 8)
	addr := 0

	for _, i := range t {
		if i.Type == assembler.TOKEN_INSTRUCTION {
			size, err := assembleInstruction(code, i)
			if err != nil {
				return code, fmt.Errorf(machine.InterCtx.Get("%v:%v: Error assembling: %v"), *i.File, i.Line, err)
			}

			addr += size
		} else {
			for _, c := range []uint8(i.Value) {
				code = append(code, c)
				addr++
			}
		}
	}

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
		Name: "Zilog Z80",
		// Words are actually 8-bit. We use 16 as that is the address size
		WordWidth: 16,
		RegistersNames: []string{
			"A", "F", "AF",
			"B", "C", "BC",
			"D", "E", "DE",
			"H", "L", "HL",
			"A'", "F'", "AF'",
			"B'", "C'", "BC'",
			"D'", "E'", "DE'",
			"H'", "L'", "HL'",
			"I", "R", "IX", "IY",
			"PC", "SP",
		},
	}
}
