package codegen

import . "luago/compiler/ast"
import . "luago/compiler/lexer"
import . "luago/lua/vm"

// kind of operands
const (
	ARG_CONST = iota
	ARG_REG
	ARG_UPVAL
	ARG_GLOBAL
)

var arithAndBitwiseBinops = map[int]int{
	TOKEN_OP_ADD:  OP_ADD,
	TOKEN_OP_SUB:  OP_SUB,
	TOKEN_OP_MUL:  OP_MUL,
	TOKEN_OP_MOD:  OP_MOD,
	TOKEN_OP_POW:  OP_POW,
	TOKEN_OP_DIV:  OP_DIV,
	TOKEN_OP_IDIV: OP_IDIV,
	TOKEN_OP_BAND: OP_BAND,
	TOKEN_OP_BOR:  OP_BOR,
	TOKEN_OP_BXOR: OP_BXOR,
	TOKEN_OP_SHL:  OP_SHL,
	TOKEN_OP_SHR:  OP_SHR,
}

func (self *cg) exp(node Exp, a, n int) {
	switch exp := node.(type) {
	case *NilExp:
		self.loadNil(exp.Line, a, n)
	case *FalseExp:
		self.loadBool(exp.Line, a, 0)
	case *TrueExp:
		self.loadBool(exp.Line, a, 1)
	case *IntegerExp:
		self.loadK(exp.Line, a, exp.Val)
	case *FloatExp:
		self.loadK(exp.Line, a, exp.Val)
	case *StringExp:
		self.loadK(exp.Line, a, exp.Str)
	case *TableConstructorExp:
		self.tcExp(exp, a)
	case *FuncDefExp:
		self.funcDefExp(exp, a)
	case *VarargExp:
		self.vararg(exp.Line, a, n)
	case *FuncCallExp:
		self.funcCallExp(exp, a, n)
	case *NameExp:
		self.nameExp(exp, a)
	case *ParensExp:
		self.exp(exp.Exp, a, 1)
	case *BracketsExp:
		self.bracketsExp(exp, a)
	case *BinopExp:
		self.binopExp(exp, a, n)
	case *UnopExp:
		self.unopExp(exp, a)
	default:
		panic("todo!")
	}
}

func (self *cg) tcExp(exp *TableConstructorExp, a int) {
	nArr := exp.NArr
	nRec := len(exp.KeyExps) - nArr
	self.newTable(exp.Line, a, nArr, nRec)

	for i, keyExp := range exp.KeyExps {
		valExp := exp.ValExps[i]

		if nArr > 0 {
			if _, ok := keyExp.(int); ok {
				tmp := self.allocTmp()
				self.exp(valExp, tmp, 1)
				continue
			}
		}

		nTmps := 0
		iKey, tKey := self.toOperand(keyExp)
		if tKey != ARG_CONST && tKey != ARG_REG {
			iKey = self.allocTmp()
			nTmps++
			self.exp(keyExp, iKey, 1)
		}

		iVal, tVal := self.toOperand(valExp)
		if tVal != ARG_CONST && tVal != ARG_REG {
			iVal = self.allocTmp()
			nTmps++
			self.exp(valExp, iVal, 1)
		}
		self.freeTmps(nTmps)
		self.setTable(lineOfExp(valExp), a, iKey, iVal)
	}

	if nArr > 0 {
		self.freeTmps(nArr)
		self.inst(exp.LastLine, OP_SETLIST, a, nArr, 1) // todo
	}
}

func (self *cg) funcDefExp(exp *FuncDefExp, a int) {
	bx := self.genSubProto(exp)
	self.closure(exp.LastLine, a, bx)
}

func (self *cg) funcCallExp(exp *FuncCallExp, a, n int) {
	self._funcCallExp(exp, a, n, false)
}

func (self *cg) tailCallExp(exp *FuncCallExp, a int) {
	self._funcCallExp(exp, a, 0, true)
}

// todo: rename?
func (self *cg) _funcCallExp(exp *FuncCallExp, a, n int, tailCall bool) {
	nArgs := len(exp.Args)
	lastArgIsVarargOrFuncCall := false

	self.loadFunc(exp.PrefixExp, a)
	for i, arg := range exp.Args {
		tmp := self.allocTmp()
		if i == nArgs-1 && isVarargOrFuncCallExp(arg) {
			lastArgIsVarargOrFuncCall = true
			self.exp(arg, tmp, -1)
		} else {
			self.exp(arg, tmp, 1)
		}
	}
	self.freeTmps(nArgs)

	if lastArgIsVarargOrFuncCall {
		nArgs = -1
	}
	if tailCall {
		self.tailCall(exp.Line, a, nArgs)
	} else {
		self.call(exp.Line, a, nArgs, n)
	}
}

func (self *cg) nameExp(exp *NameExp, a int) {
	if slot := self.slotOf(exp.Name); slot >= 0 {
		self.move(exp.Line, a, slot)
	} else if idx := self.lookupUpval(exp.Name); idx >= 0 {
		self.getUpval(exp.Line, a, idx)
	} else { // x => _ENV['x']
		bracketsExp := &BracketsExp{
			Line:      exp.Line,
			PrefixExp: &NameExp{exp.Line, "_ENV"},
			KeyExp:    &StringExp{exp.Line, exp.Name},
		}
		self.bracketsExp(bracketsExp, a)
	}
}

// prefix[key]
func (self *cg) bracketsExp(exp *BracketsExp, a int) {
	iTab, tTab := self.toOperand(exp.PrefixExp)
	iKey, tKey := self.toOperand(exp.KeyExp)

	if tTab != ARG_REG && tTab != ARG_UPVAL {
		iTab = a
		self.exp(exp.PrefixExp, iTab, 1)
	}
	if tKey != ARG_REG && tKey != ARG_CONST {
		if tTab != ARG_REG && tTab != ARG_UPVAL {
			iKey = self.allocTmp()
			self.exp(exp.KeyExp, iKey, 1)
			self.freeTmp()
		} else {
			iKey = a
			self.exp(exp.KeyExp, iKey, 1)
		}
	}

	if tTab == ARG_UPVAL {
		self.getTabUp(exp.Line, a, iTab, iKey)
	} else {
		self.getTable(exp.Line, a, iTab, iKey)
	}
}

func (self *cg) binopExp(exp *BinopExp, a, n int) {
	switch exp.Op {
	case TOKEN_OP_POW:
		self.powExp(exp, a)
	case TOKEN_OP_CONCAT:
		self.concatExp(exp, a)
	case TOKEN_OP_OR:
		self.logicalOrExp(exp, a)
	case TOKEN_OP_AND:
		self.logicalAndExp(exp, a)
	default:
		self.simpleBinopExp(exp, a, n)
	}
}

// todo: name
func (self *cg) simpleBinopExp(exp *BinopExp, a, n int) {
	iX, tX := self.toOperand(exp.Exp1)
	if tX != ARG_REG && tX != ARG_CONST {
		iX = a
		self.exp(exp.Exp1, iX, 1)
	}

	iY, tY := self.toOperand(exp.Exp2)
	if tY != ARG_REG && tY != ARG_CONST {
		if tX != ARG_REG && tX != ARG_CONST {
			iY = self.allocTmp()
			self.exp(exp.Exp2, iY, 1)
			self.freeTmp()
		} else {
			iY = a
			self.exp(exp.Exp2, iY, 1)
		}
	}

	// todo
	if opcode, ok := arithAndBitwiseBinops[exp.Op]; ok {
		self.inst(exp.Line, opcode, a, iX, iY)
		return
	}

	// relational ops
	if n == 0 {
		switch exp.Op {
		case TOKEN_OP_EQ:
			self.inst(exp.Line, OP_EQ, 0, iX, iY)
		case TOKEN_OP_NE:
			self.inst(exp.Line, OP_EQ, 1, iX, iY)
		case TOKEN_OP_LT:
			self.inst(exp.Line, OP_LT, 0, iX, iY)
		case TOKEN_OP_GT:
			self.inst(exp.Line, OP_LT, 0, iY, iX)
		case TOKEN_OP_LE:
			self.inst(exp.Line, OP_LE, 0, iX, iY)
		case TOKEN_OP_GE:
			self.inst(exp.Line, OP_LE, 0, iY, iX)
		}
	} else {
		switch exp.Op {
		case TOKEN_OP_EQ:
			self.inst(exp.Line, OP_EQ, 1, iX, iY)
		case TOKEN_OP_NE:
			self.inst(exp.Line, OP_EQ, 0, iX, iY)
		case TOKEN_OP_LT:
			self.inst(exp.Line, OP_LT, 1, iX, iY)
		case TOKEN_OP_GT:
			self.inst(exp.Line, OP_LT, 1, iY, iX)
		case TOKEN_OP_LE:
			self.inst(exp.Line, OP_LE, 1, iX, iY)
		case TOKEN_OP_GE:
			self.inst(exp.Line, OP_LE, 1, iY, iX)
		}
		self.inst(exp.Line, OP_JMP, 0, 1, 0)
		self.inst(exp.Line, OP_LOADBOOL, a, 0, 1)
		self.inst(exp.Line, OP_LOADBOOL, a, 1, 0)
	}
}

func (self *cg) powExp(exp *BinopExp, a int) {
	operands := make([]int, 0, 4) // todo: rename

	nTmps := -1
	for {
		xx := 0 // todo: rename
		iX, tX := self.toOperand(exp.Exp1)
		if tX != ARG_CONST && tX != ARG_REG {
			if nTmps < 0 {
				iX = a
				nTmps = 0
			} else {
				iX = self.allocTmp()
				nTmps++
				xx = 1
			}
			self.exp(exp.Exp1, iX, 1)
		}
		operands = append(operands, iX, xx, exp.Line)

		exp2, ok := exp.Exp2.(*BinopExp)
		if ok && exp2.Op == TOKEN_OP_POW {
			exp = exp2
		} else {
			yy := 0
			iY, tY := self.toOperand(exp.Exp2)
			if tY != ARG_CONST && tY != ARG_REG {
				if nTmps < 0 {
					iY = a
					nTmps = 0
				} else {
					iY = self.allocTmp()
					nTmps++
					yy = 1
				}
				self.exp(exp.Exp2, iY, 1)
			}
			operands = append(operands, iY, yy)

			break
		}
	}

	if nTmps > 0 {
		self.freeTmps(nTmps)
	}

	for i := len(operands) - 1; i > 3; i -= 3 {
		//yy := operands[i]
		iy := operands[i-1]
		line := operands[i-2]
		xx := operands[i-3]
		ix := operands[i-4]
		if xx == 1 {
			self.inst(line, OP_POW, ix, ix, iy)
		} else {
			self.inst(line, OP_POW, a, ix, iy)
			operands[i-4] = a
		}
	}
}

func (self *cg) concatExp(exp *BinopExp, a int) {
	nTmps := -1
	for {
		if nTmps < 0 {
			nTmps = 0
			self.exp(exp.Exp1, a, 1)
		} else {
			tmp := self.allocTmp()
			nTmps++
			self.exp(exp.Exp1, tmp, 1)
		}

		exp2, ok := exp.Exp2.(*BinopExp)
		if ok && exp2.Op == TOKEN_OP_CONCAT {
			exp = exp2
		} else {
			tmp := self.allocTmp()
			nTmps++
			self.exp(exp.Exp2, tmp, 1)
			break
		}
	}

	self.freeTmps(nTmps)
	self.inst(exp.Line, OP_CONCAT, a, a, a+nTmps)
}

func (self *cg) logicalOrExp(exp *BinopExp, a int) {
	if b, ok := self.isLocVar(exp.Exp1); ok {
		self.inst(exp.Line, OP_TESTSET, a, b, 1)
	} else {
		self.exp(exp.Exp1, a, 1)
		self.inst(exp.Line, OP_TEST, a, 0, 1)
	}

	jmpFrom := self.inst(exp.Line, OP_JMP, 0, 0, 0)
	self.exp(exp.Exp2, a, 1)
	jmpTo := self.pc() - 1

	self.fixSbx(jmpFrom, jmpTo-jmpFrom-1)
}

func (self *cg) logicalAndExp(exp *BinopExp, a int) {
	if b, ok := self.isLocVar(exp.Exp1); ok {
		self.inst(exp.Line, OP_TESTSET, a, b, 0)
	} else {
		self.exp(exp.Exp1, a, 1)
		self.inst(exp.Line, OP_TEST, a, 0, 0)
	}
	jmpFrom := self.inst(exp.Line, OP_JMP, 0, 0, 0)
	self.exp(exp.Exp2, a, 1)
	jmpTo := self.pc() - 1
	// if 0/*&LOR_LEFT*/ > 0 { // todo!
	// 	jmpTo += 2
	// }
	self.fixSbx(jmpFrom, jmpTo-jmpFrom-1)
}

func (self *cg) unopExp(exp *UnopExp, a int) {
	b := self.allocTmp()
	self.exp(exp.Exp, b, 1)
	self.freeTmp()

	switch exp.Op {
	case TOKEN_OP_NOT:
		self.inst(exp.Line, OP_NOT, a, b, 0)
	case TOKEN_OP_BNOT:
		self.inst(exp.Line, OP_BNOT, a, b, 0)
	case TOKEN_OP_LEN:
		self.inst(exp.Line, OP_LEN, a, b, 0)
	case TOKEN_OP_UNM:
		self.inst(exp.Line, OP_UNM, a, b, 0)
	}
}

func (self *cg) loadFunc(pexp Exp, tmp int) {
	if nameExp, ok := pexp.(*NameExp); ok {
		if slot := self.slotOf(nameExp.Name); slot >= 0 {
			self.inst(nameExp.Line, OP_MOVE, tmp, slot, 0)
			return
		}
	}
	self.exp(pexp, tmp, 1)
}

func (self *cg) isLocVar(exp Exp) (int, bool) {
	if nameExp, ok := exp.(*NameExp); ok {
		if slot := self.slotOf(nameExp.Name); slot >= 0 {
			return slot, true
		}
	}
	return -1, false
}

func (self *cg) isUpval(exp Exp) (int, bool) {
	if nameExp, ok := exp.(*NameExp); ok {
		if idx := self.lookupUpval(nameExp.Name); idx >= 0 {
			return idx, true
		}
	}
	return -1, false
}

// todo: rename
func (self *cg) toOperand0(exp Exp) (int, int) {
	switch x := exp.(type) {
	case *NilExp, *FalseExp, *TrueExp,
		*IntegerExp, *FloatExp, *StringExp,
		*VarargExp:
		return -1, ARG_CONST
	case *NameExp:
		if slot := self.slotOf(x.Name); slot >= 0 {
			return slot, ARG_REG
		} else if idx := self.lookupUpval(x.Name); idx >= 0 {
			return idx, ARG_UPVAL
		} else {
			return self.indexOf(x.Name), ARG_GLOBAL
		}
	}
	return -1, -1
}

func (self *cg) toOperand(exp Exp) (int, int) {
	switch x := exp.(type) {
	case *NilExp:
		return self.indexOf(nil), ARG_CONST
	case *FalseExp:
		return self.indexOf(false), ARG_CONST
	case *TrueExp:
		return self.indexOf(true), ARG_CONST
	case *IntegerExp:
		return self.indexOf(x.Val), ARG_CONST
	case *FloatExp:
		return self.indexOf(x.Val), ARG_CONST
	case *StringExp:
		return self.indexOf(x.Str), ARG_CONST
	case *NameExp:
		if slot := self.slotOf(x.Name); slot >= 0 {
			return slot, ARG_REG
		} else if idx := self.lookupUpval(x.Name); idx >= 0 {
			return idx, ARG_UPVAL
		} else {
			self.indexOf(x.Name)
			return -1, ARG_GLOBAL
		}
	}
	return -1, -1
}