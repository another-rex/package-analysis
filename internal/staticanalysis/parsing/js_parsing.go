package parsing

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os/exec"
	"strings"

	"github.com/ossf/package-analysis/internal/log"
	"github.com/ossf/package-analysis/internal/staticanalysis/token"
)

// parserOutputElement represents the output JSON format of the JS parser
type parserOutputElement struct {
	SymbolType    SymbolType     `json:"type"`
	SymbolSubtype string         `json:"subtype"`
	Data          any            `json:"data"`
	Pos           [2]int         `json:"pos"`
	Extra         map[string]any `json:"extra"`
}

/*
syntaxErrorExitCode is the exit code that the parser will return if it encounters a
syntax error while parsing the input. This also ends up being the signal of whether a given
input is JavaScript or not - without an external tool that detects file types, it's hard
to tell between 'JavaScript with a few errors' and 'a totally non-JavaScript file'.
*/
const syntaxErrorExitCode = 33

/*
runParser handles calling the parser program and provide the specified Javascript source to it,
either by filename (jsFilePath) or piping jsSource to the program's stdin.
If sourcePath is empty, sourceString will be parsed as JS code
*/
func runParser(parserPath, jsFilePath, jsSource string) (string, error) {
	nodeArgs := []string{parserPath}
	if len(jsFilePath) > 0 {
		nodeArgs = append(nodeArgs, jsFilePath)
	}

	cmd := exec.Command("node", nodeArgs...)

	if len(jsFilePath) == 0 {
		// create a pipe to send the source code to the parser via stdin
		pipe, pipeErr := cmd.StdinPipe()
		if pipeErr != nil {
			return "", fmt.Errorf("runParser failed to create pipe: %v", pipeErr)
		}

		if _, pipeErr = pipe.Write([]byte(jsSource)); pipeErr != nil {
			return "", fmt.Errorf("runParser failed to write source string to pipe: %w", pipeErr)
		}

		if pipeErr = pipe.Close(); pipeErr != nil {
			return "", fmt.Errorf("runParser failed to close pipe: %w", pipeErr)
		}
	}

	out, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(out), nil
}

/*
parseJS extracts source code identifiers and string literals from JavaScript code.
If sourcePath is empty, sourceString will be parsed as JS code.

parserConfig specifies options relevant to the parser itself, and is produced by InitParser

If the input contains a syntax error (which could mean it's not actually JavaScript),
then a pointer to parsing.InvalidInput is returned.
*/
func parseJS(parserConfig ParserConfig, filePath string, sourceString string) (result parserOutput, parserOutput string, err error) {
	parserOutput, err = runParser(parserConfig.ParserPath, filePath, sourceString)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == syntaxErrorExitCode {
				return InvalidInput, "", nil
			}
			parserOutput = string(exitErr.Stderr)
		}
		return
	}

	// parse JSON to get results as Go struct
	decoder := json.NewDecoder(strings.NewReader(parserOutput))
	var storage []parserOutputElement
	err = decoder.Decode(&storage)
	if err != nil {
		return
	}

	result.ValidInput = true

	// convert the elements into more natural data structure
	for _, element := range storage {
		switch element.SymbolType {
		case Identifier:
			symbolSubtype := token.CheckIdentifierType(element.SymbolSubtype)
			if symbolSubtype == token.Other || symbolSubtype == token.Unknown {
				break
			}
			result.Identifiers = append(result.Identifiers, parsedIdentifier{
				Type: token.CheckIdentifierType(element.SymbolSubtype),
				Name: element.Data.(string),
				Pos:  element.Pos,
			})
		case Literal:
			literal := parsedLiteral[any]{
				Type:     element.SymbolSubtype,
				GoType:   fmt.Sprintf("%T", element.Data),
				Value:    element.Data,
				RawValue: element.Extra["raw"].(string),
				InArray:  element.Extra["array"] == true,
				Pos:      element.Pos,
			}
			// check for BigInteger types which have to be represented as strings in JSON
			if literal.Type == "Numeric" && literal.GoType == "string" {
				if intAsString, ok := literal.Value.(string); ok {
					var bigInt big.Int
					if _, valid := bigInt.SetString(intAsString, 0); valid {
						literal.Value = &bigInt
						literal.GoType = fmt.Sprintf("%T", bigInt)
					}
				}
			}
			result.Literals = append(result.Literals, literal)
		case Comment:
			result.Comments = append(result.Comments, parsedComment{
				Type: element.SymbolSubtype,
				Data: element.Data.(string),
				Pos:  element.Pos,
			})
		case Info:
			fallthrough
		case Error:
			// ignore for now
		default:
			log.Warn(fmt.Sprintf("parseJS: unrecognised symbol type %s", element.SymbolType))
		}
	}
	return
}

func RunExampleParsing(config ParserConfig, jsFilePath string, jsSourceString string) {
	parseResult, parserOutput, err := parseJS(config, jsFilePath, jsSourceString)

	println("\nRaw JSON:\n", parserOutput)

	if err != nil {
		fmt.Printf("Error: %s\n", err)
		if ee, ok := err.(*exec.ExitError); ok {
			fmt.Printf("Process stderr:\n")
			fmt.Println(string(ee.Stderr))
		}
		return
	} else {
		fmt.Println("Completed without errors")
	}
	println()
	println("== Parsed Identifiers ==")
	for _, identifier := range parseResult.Identifiers {
		fmt.Printf("%v\n", identifier)
	}
	println()
	println("== Parsed Literals ==")
	for _, literal := range parseResult.Literals {
		fmt.Printf("%v\n", literal)
	}

	println()
	println("== Parsed Comments ==")
	for _, comment := range parseResult.Comments {
		fmt.Printf("%v\n", comment)
	}

}