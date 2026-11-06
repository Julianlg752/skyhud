# Installs SkyHUD for DaVinci Resolve. Run it with "Install SkyHUD.bat".
$ErrorActionPreference = "Stop"
$exe = Join-Path $PSScriptRoot "skyhud.exe"

# The zip came from the internet; let Windows run skyhud.exe.
Unblock-File -Path $exe -ErrorAction SilentlyContinue

Write-Host "Installing SkyHUD..."
& $exe install
if ($LASTEXITCODE -ne 0) { throw "skyhud install failed" }
Write-Host ""

$status = & $exe key status
if ($status -match "not stored") {
	Write-Host "Flying a DJI Avata 2? SkyHUD can add sticks and battery from your"
	Write-Host "flight logs with your own DJI API key (see docs/install.md)."
	$secure = Read-Host -AsSecureString "Paste your DJI API key and press Enter (or just press Enter to skip)"
	$bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
	try {
		$key = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr)
		if ($key) { $key | & $exe key set }
	} finally {
		[Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr)
		Remove-Variable key -ErrorAction SilentlyContinue
	}
}

Write-Host ""
Write-Host "Done. Restart DaVinci Resolve, then open Workspace > Scripts > SkyHUD."
