#Requires -Version 5.1
<#
.SYNOPSIS
Runs the #276 real-Vertex family-check smoke against a no-traffic Cloud Run tag.

.DESCRIPTION
Posts synthetic inputs to a tagged revision of the backend, checks the real
Vertex response, runs the offline guard tests for invented/malformed/model-error
answers, and writes an evidence file. It never shifts traffic or creates a tag.

.EXAMPLE
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/audit-m2-smoke.ps1

.NOTES
Prerequisites: gcloud signed in with project access, curl, Go, and an existing
no-traffic tag on the service. See docs/smoke-workflow.md.
#>
param(
    [string]$Project = 'wassup-hackathon-2026',
    [string]$Region = 'us-central1',
    [string]$Service = 'backend',
    [string]$Tag = 'audit-m2',
    [string]$EvidenceFile = (Join-Path $PSScriptRoot '..\docs\audit-m2-evidence.md'),
    [switch]$SkipOfflineTests,
    [string]$OfflineTestEvidenceUrl = ''
)

$ErrorActionPreference = 'Continue'
$ProgressPreference = 'SilentlyContinue'
Set-StrictMode -Version Latest

function Get-Gcloud {
    $cmd = Get-Command gcloud -ErrorAction SilentlyContinue
    if (-not $cmd) { throw 'gcloud is not on PATH' }
    return $cmd.Source
}

function Invoke-GcloudJson {
    param([Parameter(Mandatory)][string[]]$GcloudArgs)
    $output = & $script:Gcloud @GcloudArgs 2>$null
    if ($LASTEXITCODE -ne 0) { throw "gcloud $($GcloudArgs -join ' ') failed with exit $LASTEXITCODE" }
    return ($output | Out-String | ConvertFrom-Json)
}

function Get-EnvValue {
    param([object]$Revision, [string]$Name)
    $env = @($Revision.spec.containers[0].env)
    $hit = $env | Where-Object { $_.name -eq $Name } | Select-Object -First 1
    if ($hit) { return [string]$hit.value }
    return ''
}

function Post-FamilyCheck {
    param([string]$BaseUrl, [string]$Token, [string]$Body)
    $tmp = Join-Path ([IO.Path]::GetTempPath()) ("m2-" + [guid]::NewGuid().ToString('N') + '.json')
    [IO.File]::WriteAllText($tmp, $Body, (New-Object Text.UTF8Encoding($false)))
    try {
        $code = & curl.exe -sS -o "$tmp.out" -w '%{http_code}' --max-time 90 `
            -H "Authorization: Bearer $Token" -H 'Content-Type: application/json' `
            --data-binary "@$tmp" "$BaseUrl/v1/products/family-check"
        if ($LASTEXITCODE -ne 0) { throw "curl failed with exit $LASTEXITCODE" }
        $raw = [IO.File]::ReadAllText("$tmp.out")
        return [pscustomobject]@{ Code = [int]$code; Raw = $raw; Json = ($raw | ConvertFrom-Json) }
    }
    finally {
        Remove-Item $tmp, "$tmp.out" -Force -ErrorAction SilentlyContinue
    }
}

$script:Gcloud = Get-Gcloud

$serviceJson = Invoke-GcloudJson @('run','services','describe',$Service,'--project',$Project,'--region',$Region,'--format=json')
$traffic = @($serviceJson.status.traffic)
$route = $traffic | Where-Object { $_.tag -eq $Tag } | Select-Object -First 1
if (-not $route) {
    throw "tag '$Tag' is not on service '$Service'. Add it without shifting traffic, e.g. gcloud run services update-traffic $Service --update-tags=$Tag=LATEST --project $Project --region $Region"
}
$baseUrl = $route.url
if (-not $baseUrl) {
    $serviceHost = ([uri]$serviceJson.status.url).Host
    $baseUrl = "https://$Tag---$serviceHost"
}
$revisionName = $route.revisionName
if (-not $revisionName) { throw "tag '$Tag' has no revisionName" }

$revision = Invoke-GcloudJson @('run','revisions','describe',$revisionName,'--project',$Project,'--region',$Region,'--format=json')
$image = [string]$revision.spec.containers[0].image
$serviceAccount = [string]$revision.spec.serviceAccountName
$created = [string]$revision.metadata.creationTimestamp
$provider = Get-EnvValue $revision 'LLM_PROVIDER'
$routerModel = Get-EnvValue $revision 'ROUTER_MODEL'
if (-not $routerModel) { $routerModel = 'gemini-3.8-flash' } # config.go default
$googleLocation = Get-EnvValue $revision 'GOOGLE_CLOUD_LOCATION'
$googleProject = Get-EnvValue $revision 'GOOGLE_CLOUD_PROJECT'

$token = & $script:Gcloud auth print-identity-token 2>$null
if ($LASTEXITCODE -ne 0 -or -not $token) { throw 'could not get a Google identity token' }

$results = @()

$supported = Post-FamilyCheck $baseUrl $token '{"text":"hair dryer with a motor, heating coil and moulded plastic housing"}'
if ($supported.Code -ne 200 -or [string]$supported.Json.llm_provider -ne 'vertex' -or
    [string]$supported.Json.result.family -in @('', 'none')) {
    throw "supported case failed: $($supported.Code) $($supported.Raw)"
}
$results += [pscustomobject]@{ Case = 'supported synthetic hair dryer'; Status = $supported.Code; Family = [string]$supported.Json.result.family; Provider = [string]$supported.Json.llm_provider; Body = $supported.Raw }

$unsupported = Post-FamilyCheck $baseUrl $token '{"text":"synthetic sailing yacht hull made of fibreglass"}'
if ($unsupported.Code -ne 200 -or [string]$unsupported.Json.result.family -ne 'none') {
    throw "unsupported case failed: $($unsupported.Code) $($unsupported.Raw)"
}
$results += [pscustomobject]@{ Case = 'unsupported synthetic yacht hull'; Status = $unsupported.Code; Family = [string]$unsupported.Json.result.family; Provider = [string]$unsupported.Json.llm_provider; Body = $unsupported.Raw }

$malformed = Post-FamilyCheck $baseUrl $token '{"text":"   "}'
if ($malformed.Code -ne 400 -or [string]$malformed.Json.code -ne 'invalid_input') {
    throw "malformed request case failed: $($malformed.Code) $($malformed.Raw)"
}
$results += [pscustomobject]@{ Case = 'malformed request (blank text)'; Status = $malformed.Code; Family = '-'; Provider = '-'; Body = $malformed.Raw }

$offlineCommand = "GOFLAGS=-mod=readonly go test -count=1 -run 'TestSchemaSentToModelListsOnlyKnownFamiliesAndNone|TestInventedFamilyAndInvalidEnumAreRejected|TestMalformedModelOutputFailsClosed|TestProviderErrorsFailClosedWithoutRetry' ./internal/readiness/smokecheck/"
if ($SkipOfflineTests) {
    if (-not $OfflineTestEvidenceUrl) {
        throw '-SkipOfflineTests needs -OfflineTestEvidenceUrl with a passing run of .github/workflows/smoke-nonprod.yml'
    }
    $offlineNote = @(
        'Skipped locally: this machine blocks execution of Go-built test binaries by application control.',
        "Evidence: $OfflineTestEvidenceUrl",
        '',
        'Command run by the passing workflow:',
        $offlineCommand
    ) -join "`n"
}
else {
    $testArgs = @('test','-count=1','-run','TestSchemaSentToModelListsOnlyKnownFamiliesAndNone|TestInventedFamilyAndInvalidEnumAreRejected|TestMalformedModelOutputFailsClosed|TestProviderErrorsFailClosedWithoutRetry','./internal/readiness/smokecheck/')
    $env:GOFLAGS = '-mod=readonly'
    $backendDir = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..\backend'))
    Push-Location $backendDir
    try {
        $testOut = & go @testArgs 2>&1
        $testCode = $LASTEXITCODE
    }
    finally {
        Pop-Location
    }
    $testText = ($testOut | ForEach-Object { $_.ToString() }) -join "`n"
    if ($testCode -ne 0) { throw "offline guard tests failed:`n$testText" }
    $offlineNote = @($offlineCommand, $testText.Trim()) -join "`n"
}

$lines = @()
$lines += '# Audit M2 evidence: real-Vertex family-check smoke (#276)'
$lines += ''
$lines += "Run at: $((Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ'))"
$lines += ''
$lines += '## Target'
$lines += ''
$lines += '| Field | Value |'
$lines += '| --- | --- |'
$lines += "| Project / region | ``$Project`` / ``$Region`` |"
$lines += "| Service | ``$Service`` |"
$lines += "| No-traffic tag | ``$Tag`` |"
$lines += "| Tag URL | ``$baseUrl`` |"
$lines += "| Revision | ``$revisionName`` (created $created) |"
$lines += "| Image | ``$image`` |"
$lines += "| Service account | ``$serviceAccount`` |"
$lines += "| Provider / model | ``$provider`` / ``$routerModel`` |"
$lines += "| Location / project env | ``$googleLocation`` / ``$googleProject`` |"
$lines += ''
$lines += 'The tag URL addresses the revision directly. No traffic percentage was shifted by this run; the untagged service URL remains the live route.'
$lines += ''
$lines += '## Live cases against real Vertex'
$lines += ''
$lines += 'All inputs are synthetic. Tokens are not recorded.'
$lines += ''
$lines += '| Case | HTTP | Family | Provider | Response |'
$lines += '| --- | ---: | --- | --- | --- |'
foreach ($r in $results) {
    $body = ($r.Body -replace '\r?\n', ' ') -replace '\|', '\|'
    $lines += "| $($r.Case) | $($r.Status) | $($r.Family) | $($r.Provider) | ``$body`` |"
}
$lines += ''
$lines += '## Offline guard evidence'
$lines += ''
$lines += 'Native Vertex cannot be forced to emit a bad enum, malformed JSON or a provider error on demand. Those paths stay covered by the existing loopback tests with a fake model:'
$lines += ''
$lines += '```text'
$lines += $offlineNote
$lines += '```'
$lines += ''
$lines += 'These tests assert the schema enum contains only known template families plus `none`, invented or malformed families return `502 model_bad_response` with no result and no retry, and provider errors return `502 model_unavailable` without echoing upstream details.'
$lines += ''
$lines += '## Promotion gate'
$lines += ''
$lines += 'Before production traffic moves to a new revision, a person must run this script against a no-traffic tag of that revision and keep the generated evidence. Promotion is blocked when any live case, guard test, or recorded revision/model/config check fails. Mocked tests alone never satisfy this gate.'

$fullPath = [IO.Path]::GetFullPath($EvidenceFile)
[IO.File]::WriteAllLines($fullPath, $lines, (New-Object Text.UTF8Encoding($false)))
Write-Host "PASS: real-Vertex smoke against $baseUrl"
Write-Host "Evidence written to $fullPath"
