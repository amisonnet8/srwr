# PowerShell completion for srwr.
#
# It has no list of commands. At every TAB it asks `srwr __complete` what can
# come next, so it is never out of date with the srwr that is installed, and
# it can offer the tape IDs of the workspace.
#
# Install it with:  srwr completion powershell >> $PROFILE
# or try it with:   srwr completion powershell | Out-String | Invoke-Expression
Register-ArgumentCompleter -Native -CommandName srwr -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)

    # The words before the one being typed, without "srwr": the elements of
    # the line that end before the cursor.
    $words = @($commandAst.CommandElements | Select-Object -Skip 1 |
        Where-Object { $_.Extent.EndOffset -lt $cursorPosition } |
        ForEach-Object {
            if ($_ -is [System.Management.Automation.Language.StringConstantExpressionAst]) { $_.Value } else { $_.Extent.Text }
        })
    # The word being typed, without its quotes, as --word: an empty word would
    # be dropped, and "--word=" never is.
    $word = $wordToComplete -replace '^[''"]|[''"]$', ''

    $exitCode = $global:LASTEXITCODE
    $lines = srwr __complete "--word=$word" -- @words 2>$null
    $global:LASTEXITCODE = $exitCode

    foreach ($line in $lines) {
        if (-not $line) { continue }
        $value, $description = $line -split "`t", 2
        # A completion result must have a description to show.
        if (-not $description) { $description = $value }
        [System.Management.Automation.CompletionResult]::new($value, $value, 'ParameterValue', $description)
    }
}
