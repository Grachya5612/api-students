function Call-Api {
    param($Method, $Path, $Token, $Body = @{})

    $params = @{
        Method          = $Method
        Uri             = "$API$Path"
        Headers         = @{ Authorization = "Bearer $Token"; Accept = "application/json" }
        ContentType     = "application/json"
        Body            = ($Body | ConvertTo-Json -Compress)
        UseBasicParsing = $true
    }

    try {
        $r = Invoke-WebRequest @params
        return "$($r.StatusCode) $($r.Content)"
    } catch {
        $resp = $_.Exception.Response
        if ($null -eq $resp) { return "ERROR $($_.Exception.Message)" }
        $code = [int]$resp.StatusCode
        $reader = New-Object System.IO.StreamReader($resp.GetResponseStream())
        return "$code $($reader.ReadToEnd())"
    }
}