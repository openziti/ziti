function Start-UDP-Server {
  [CmdletBinding()]
  param (
    # The UDP port to listen on. Default 10000
    [Parameter(Mandatory = $false)]
    $Port = 10000,

    # Should the test server echo back the bytes sent
    [Parameter(Mandatory = $false)]
    $Echo = $false
  )
  $Echo = "${Echo}" -eq "${true}"
  
  $udpListener = new-Object system.Net.Sockets.Udpclient($Port)
  $udpListener.Client.ReceiveTimeout = 100
  $encoding = New-Object System.Text.ASCIIEncoding
  Try {
    "Starting UDP Listener on port {0}:" -f $Port
    
    do {
      $remoteInterface = New-Object system.net.ipendpoint([system.net.ipaddress]::Any,0)
      Try {
        $receivebytes = $udpListener.Receive([ref]$remoteInterface)      
      } Catch {
        #Write-Warning "$($Error[0])"
      }  
      If ($receivebytes) {
          [string]$msg = $encoding.GetString($receivebytes)
          $from = "{0}:{1}" -f $remoteInterface.Address,$remoteInterface.Port
          $out = "{0}{1}" -f $from.PadRight(20),$msg
          
          if($Echo) {
            $udpListener.Connect($remoteInterface.Address, $remoteInterface.Port)
            $msg="message received and returned: {0}" -f $msg
            $receivebytes=$encoding.GetBytes($msg)
            [void]$udpListener.Send($receivebytes, $receivebytes.length)
          }
          $receivebytes = ""
          Write-Host -NoNewline $out
      } Else {
          #"No data received ...
      }
    } while (1)   
  } Catch {
    #Write-Warning "$($Error[0])"
  } Finally {
    Write-Host "Stopping UDP listener on ${Port}..."
    $udpListener.Close()
    $responder.Close()
  }
}

function Test-UDP-Client {
  [CmdletBinding()]
  param (
    # The IP address to send UDP traffic to.
    [Parameter(Mandatory = $true)]
    $Server,

    # The UDP port to listen on. Default 10000
    [Parameter(Mandatory = $false)]
    $Port = 10000
  )
  
  $udpClient = new-Object system.Net.Sockets.Udpclient
  $udpClient.Connect($Server, $Port)
  $encoding = New-Object System.Text.ASCIIEncoding
  Try {
      do {
        $msg = Read-Host
        $receivebytes=$encoding.GetBytes($msg+"`n")
        [void]$udpClient.Send($receivebytes, $receivebytes.length)
      } while(1)
  } Catch {
    Write-Warning "$($Error[0])"
  } Finally {
    Write-Host "stopping..."
    $udpClient.Close()
  }
}