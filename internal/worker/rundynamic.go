package worker

import (
	"github.com/ossf/package-analysis/internal/analysis"
	"github.com/ossf/package-analysis/internal/dynamicanalysis"
	"github.com/ossf/package-analysis/internal/pkgecosystem"
	"github.com/ossf/package-analysis/internal/sandbox"
)

type DynamicAnalysisStraceSummary map[pkgecosystem.RunPhase]*dynamicanalysis.StraceSummary
type DynamicAnalysisFileWrites map[pkgecosystem.RunPhase]*dynamicanalysis.FileWrites
type DynamicAnalysisResults struct {
	StraceSummary DynamicAnalysisStraceSummary
	FileWrites    DynamicAnalysisFileWrites
}

/*
RunDynamicAnalysis runs dynamic analysis on the given package in the sandbox
provided, across all phases (e.g. import, install) valid in the package ecosystem.
Status and errors are logged to stdout. There are 4 return values:

DynamicAnalysisResults: Map of each successfully run phase to a summary of
the corresponding dynamic analysis result. This summary has two parts:
1. StraceSummary: information about system calls performed by the process
2. FileWrites: list of files which were written to and counts of bytes written

Note, if error is not nil, then results[lastRunPhase] is nil.

RunPhase: the last phase that was run. If error is non-nil, this phase did not
successfully complete, and the results for this phase are not recorded.
Otherwise, the results contain data for this phase, even in cases where the
sandboxed process terminated abnormally.

Status: the status of the last run phase if it completed without error, else empty

error: Any error that occurred in the runtime/sandbox infrastructure.
This does not include errors caused by the package under analysis.
*/

func RunDynamicAnalysis(sb sandbox.Sandbox, pkg *pkgecosystem.Pkg) (DynamicAnalysisResults, pkgecosystem.RunPhase, analysis.Status, error) {
	results := DynamicAnalysisResults{
		StraceSummary: make(DynamicAnalysisStraceSummary),
		FileWrites:    make(DynamicAnalysisFileWrites),
	}

	var lastRunPhase pkgecosystem.RunPhase
	var lastStatus analysis.Status
	var lastError error
	for _, phase := range pkg.Manager().RunPhases() {
		result, err := dynamicanalysis.Run(sb, pkg.Command(phase))
		lastRunPhase = phase

		if err != nil {
			// Error when trying to actually run; don't record the result for this phase
			// or attempt subsequent phases
			lastStatus = ""
			lastError = err
			break
		}

		results.StraceSummary[phase] = &result.StraceSummary
		results.FileWrites[phase] = &result.FileWrites
		lastStatus = result.StraceSummary.Status

		if lastStatus != analysis.StatusCompleted {
			// Error caused by an issue with the package (probably).
			// Don't continue with phases if this one did not complete successfully.
			break
		}
	}

	if lastError != nil {
		LogDynamicAnalysisError(pkg, lastRunPhase, lastError)
	} else {
		LogDynamicAnalysisResult(pkg, lastRunPhase, lastStatus)
	}

	return results, lastRunPhase, lastStatus, lastError
}