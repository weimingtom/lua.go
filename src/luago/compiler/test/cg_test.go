package test

import "fmt"
import "strings"
import "testing"
import "assert"
import "luago/compiler"

func TestFuncCallStat(t *testing.T) {
	testInsts(t, "f()", "[2/0] gettabup(0,0,-1); call(0,1,1)")
	testInsts(t, "f(1,2)", "[3/0] gettabup(0,0,-1); loadk(1,-2); loadk(2,-3); call(0,3,1)")
	testInsts(t, "f(1,g(2,h(3)))",
`[6/0]
gettabup(0,0,-1); loadk(1,-2);
gettabup(2,0,-3); loadk(3,-4);
gettabup(4,0,-5); loadk(5,-6);
call(4,2,0); call(2,0,0); call(0,0,1)`)
}

func TestRepeatStat(t *testing.T) {
	testInsts(t, "repeat f() until g()",
`[2/0]
gettabup(0,0,-1); call(0,1,1);
gettabup(0,0,-2); call(0,1,2);
test(0,_,0); jmp(0,-6)`)
}

func TestWhileStat(t *testing.T) {
	testInsts(t, "while f() do g() end",
`[2/0]
gettabup(0,0,-1); call(0,1,2);
test(0,_,0); jmp(0,3);
gettabup(0,0,-2); call(0,1,1);
jmp(0,-7)`)
}

func TestIfStat(t *testing.T) {
	testInsts(t, "if a then f() elseif b then g() end",
`[2/0]
gettabup(0,0,-1); test(0,_,0); jmp(0,3);
gettabup(0,0,-2); call(0,1,1); jmp(0,5);
gettabup(0,0,-3); test(0,_,0); jmp(0,2);
gettabup(0,0,-4); call(0,1,1)`)
}

func TestForNumStat(t *testing.T) {
	testInsts(t, "for i=1,100,2 do f() end",
`[5/4]
loadk(0,-1);
loadk(1,-2);
loadk(2,-3);
forprep(0,2);
gettabup(4,0,-4);
call(4,1,1);
forloop(0,-3)`)
}

func TestForInStat(t *testing.T) {
	testInsts(t, "for k,v in pairs(t) do print(k,v) end",
`[8/5]
gettabup(0,0,-1);
gettabup(1,0,-2);
call(0,2,4);
jmp(0,4);
gettabup(5,0,-3);
move(6,3,_);
move(7,4,_);
call(5,3,1);
tforcall(0,_,2);
tforloop(2,-6)`)
}

func TestLocalAssignStat(t *testing.T) {
	testInsts(t, "local a", "[2/1] loadnil(0,0,_)")
	testInsts(t, "local a=nil", "[2/1] loadnil(0,0,_)")
	testInsts(t, "local a=true", "[2/1] loadbool(0,1,0)")
	testInsts(t, "local a=false", "[2/1] loadbool(0,0,0)")
	testInsts(t, "local a=1", "[2/1] loadk(0,-1)")
	testInsts(t, "local a='foo'", "[2/1] loadk(0,-1)")
	testInsts(t, "local a,b,c=1,2,3", "[3/3] loadk(0,-1); loadk(1,-2); loadk(2,-3)")
	testInsts(t, "local a,b,c=f()", "[3/3] gettabup(0,0,-1); call(0,1,4)")
	testInsts(t, "local a=1,nil", "[2/1] loadk(0,-1)")
	testInsts(t, "local a=1,f()", "[2/1] loadk(0,-1); gettabup(1,0,-2); call(1,1,1)")
	testInsts(t, "local a,b,c", "[3/3] loadnil(0,2,_)")
}

func TestAssignStat(t *testing.T) {
	testInsts(t, "local a; a=nil", "[2/1] loadnil(0,0,_); loadnil(1,0,_); move(0,1,_)")
	testInsts(t, "local a; a=1", "[2/1] loadnil(0,0,_); loadk(1,-1); move(0,1,_)")
	testInsts(t, "local a; a=f()", "[2/1] loadnil(0,0,_); gettabup(1,0,-1); call(1,1,2); move(0,1,_)")
	testInsts(t, "local a; a=1,f()", "[3/1] loadnil(0,0,_); loadk(1,-1); gettabup(2,0,-2); call(2,1,1); move(0,1,_)")
	testInsts(t, "local a; a=f(),1", "[3/1] loadnil(0,0,_); gettabup(1,0,-1); call(1,1,2); loadk(2,-2); move(0,1,_)")
	testInsts(t, "local a; a[1]=2", "[4/1] loadnil(0,0,_); move(1,0,_); loadk(2,-1); loadk(3,-2); settable(1,2,3)")
	testInsts(t, "a=nil", "[2/0] loadnil(0,0,_); settabup(0,-1,0)")
	testInsts(t, "a=1", "[2/0] loadk(0,-1); settabup(0,-2,0)")
}

func testInsts(t *testing.T, chunk, expected string) {
	insts := compile(chunk)
	expected = strings.Replace(expected, "\n", " ", -1)
	expected += "; return(0,1,_)"
	assert.StringEqual(t, insts, expected)
}

func compile(chunk string) string {
	proto := compiler.Compile("src", chunk)

	s := fmt.Sprintf("[%d/%d] ", proto.MaxStackSize, len(proto.LocVars))
	for i, inst := range proto.Code {
		s += instToStr(inst)
		if i < len(proto.Code)-1 {
			s += "; "
		}
	}

	return s
}
