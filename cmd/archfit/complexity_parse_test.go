package main

import "testing"

func TestParseLizardCSV(t *testing.T) {
	// nloc,ccn,token,param,length,location,file,function,long,start,end
	csv := `283,72,1352,4,337,parse_entries@401-737@/r/src/transcript_parser.py,/r/src/transcript_parser.py,parse_entries,parse_entries( self),401,737
5,3,20,1,6,small@10-15@/r/src/x.py,/r/src/x.py,small,small( a),10,15
`
	funcs := parseLizardCSV([]byte(csv), "/r")
	if len(funcs) != 2 {
		t.Fatalf("expected 2 funcs, got %d", len(funcs))
	}
	if funcs[0].Name != "parse_entries" || funcs[0].CCN != 72 || funcs[0].Line != 401 {
		t.Errorf("bad first record: %+v", funcs[0])
	}
	if funcs[0].File != "src/transcript_parser.py" {
		t.Errorf("file should be repo-relative, got %q", funcs[0].File)
	}
}
