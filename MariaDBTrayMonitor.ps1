Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing

# Configuration
$CONFIG_PATH = "C:\AredahScripts\MariaDBSwitcher"
$CONFIG_INTERNAL = "$CONFIG_PATH\my-internal.ini"
$CONFIG_EXTERNAL = "$CONFIG_PATH\my-external.ini"
$EXTERNAL_DRIVE = "Z:"
$EXTERNAL_DATA_PATH = "$EXTERNAL_DRIVE\MariaDB\data"
$MARIADB_BIN = "C:\Program Files\MariaDB 11.8\bin"

function Get-MariaDBStatus {
    $status = @{
        IsRunning = $false
        ConfigType = "Unknown"
        DataPath = "Not found"
        ProcessId = $null
    }
    
    try {
        # Check if MariaDB is running
        $process = Get-Process -Name "mysqld" -ErrorAction SilentlyContinue
        if ($process) {
            $status.IsRunning = $true
            $status.ProcessId = $process.Id
            
            # Query MariaDB directly for the actual datadir
            try {
                # Fixed: Use & operator with proper argument array
                $mysqlExe = "$MARIADB_BIN\mysql.exe"
                $result = & $mysqlExe -u root --skip-password -s -N -e "SHOW VARIABLES LIKE 'datadir';" 2>$null
                
                if ($result -and $result.Contains("`t")) {
                    # Parse the result (format: "datadir<tab>path")
                    $actualDataDir = ($result -split "`t")[1].Trim()
                    $status.DataPath = $actualDataDir
                    
                    # Determine if it's external or internal based on actual path
                    if ($actualDataDir -like "$EXTERNAL_DRIVE*") {
                        $status.ConfigType = "External"
                    } else {
                        $status.ConfigType = "Internal"
                    }
                } else {
                    # Fallback: try with different connection approach
                    Write-Host "Direct query failed, using fallback detection method"
                    $status = Get-MariaDBStatusFallback $status
                }
            } catch {
                Write-Host "MySQL query failed: $($_.Exception.Message)"
                # Fallback to file-based detection
                $status = Get-MariaDBStatusFallback $status
            }
        }
    } catch {
        Write-Host "Error checking MariaDB status: $($_.Exception.Message)"
    }
    
    return $status
}

function Get-MariaDBStatusFallback {
    param($status)
    
    try {
        # Fallback method: Check which config file process was started with
        $process = Get-Process -Name "mysqld" -ErrorAction SilentlyContinue
        if ($process) {
            # Try to get command line arguments
            $commandLine = (Get-CimInstance Win32_Process -Filter "ProcessId = $($process.Id)").CommandLine
            
            if ($commandLine -like "*$CONFIG_EXTERNAL*") {
                $status.ConfigType = "External"
                $status.DataPath = $EXTERNAL_DATA_PATH
            } elseif ($commandLine -like "*$CONFIG_INTERNAL*") {
                $status.ConfigType = "Internal"
                # Extract datadir from internal config
                if (Test-Path $CONFIG_INTERNAL) {
                    $content = Get-Content $CONFIG_INTERNAL -ErrorAction SilentlyContinue
                    $datadir = ($content | Where-Object { $_ -match "^datadir" }) -replace "datadir\s*=\s*", "" -replace '"', ''
                    if ($datadir) {
                        $status.DataPath = $datadir.Trim()
                    } else {
                        $status.DataPath = "C:\Program Files\MariaDB 11.8\data\"
                    }
                }
            } else {
                # Last resort: check if external drive exists and has data
                $externalDriveExists = Test-Path "$EXTERNAL_DRIVE\"
                $externalDataExists = Test-Path "$EXTERNAL_DATA_PATH"
                
                if ($externalDriveExists -and $externalDataExists) {
                    $status.ConfigType = "External"
                    $status.DataPath = "$EXTERNAL_DATA_PATH"
                } else {
                    $status.ConfigType = "Internal"
                    $status.DataPath = "C:\Program Files\MariaDB 11.8\data\"
                }
            }
        }
    } catch {
        Write-Host "Fallback detection failed: $($_.Exception.Message)"
        $status.ConfigType = "Unknown"
        $status.DataPath = "Detection failed"
    }
    
    return $status
}

function Stop-MariaDBService {
    try {
        $process = Get-Process -Name "mysqld" -ErrorAction SilentlyContinue
        if ($process) {
            Stop-Process -Name "mysqld" -Force -ErrorAction Stop
            Start-Sleep -Seconds 3
            [System.Windows.Forms.MessageBox]::Show("MariaDB stopped successfully.", "Success", [System.Windows.Forms.MessageBoxButtons]::OK, [System.Windows.Forms.MessageBoxIcon]::Information)
        } else {
            [System.Windows.Forms.MessageBox]::Show("MariaDB is not currently running.", "Information", [System.Windows.Forms.MessageBoxButtons]::OK, [System.Windows.Forms.MessageBoxIcon]::Information)
        }
    } catch {
        [System.Windows.Forms.MessageBox]::Show("Failed to stop MariaDB: $($_.Exception.Message)", "Error", [System.Windows.Forms.MessageBoxButtons]::OK, [System.Windows.Forms.MessageBoxIcon]::Error)
    }
}

function Test-ConfigurationFiles {
    $issues = @()
    
    if (-not (Test-Path $CONFIG_INTERNAL)) {
        $issues += "Internal config file missing: $CONFIG_INTERNAL"
    }
    
    if (-not (Test-Path $CONFIG_EXTERNAL)) {
        $issues += "External config file missing: $CONFIG_EXTERNAL"
    }
    
    return $issues
}

function Get-OptimalConfiguration {
    # This function now suggests the OPPOSITE of what's currently running (true switching)
    $result = @{
        ConfigType = "INTERNAL"
        ConfigFile = $CONFIG_INTERNAL
        DataPath = ""
        Warnings = @()
        CanSwitchToExternal = $false
        CanSwitchToInternal = $false
    }
    
    # Get current status
    $currentStatus = Get-MariaDBStatus
    
    # Check if external configuration is available
    $externalDriveExists = Test-Path "$EXTERNAL_DRIVE\"
    $externalDataExists = $false
    $externalHasCriticalFiles = $false
    
    if ($externalDriveExists) {
        Write-Host "External drive Z: found"
        $externalDataExists = Test-Path $EXTERNAL_DATA_PATH
        
        if ($externalDataExists) {
            Write-Host "External data directory found"
            
            # Check for essential database files
            $mysqlExists = Test-Path "$EXTERNAL_DATA_PATH\mysql"
            $ibdataExists = Test-Path "$EXTERNAL_DATA_PATH\ibdata1"
            
            if ($mysqlExists) {
                Write-Host "MySQL system database found"
                $externalHasCriticalFiles = $true
                $result.CanSwitchToExternal = $true
            } else {
                $result.Warnings += "MySQL system database missing in external data"
            }
            
            if ($ibdataExists) {
                Write-Host "InnoDB data file found"
            } else {
                $result.Warnings += "InnoDB data file missing in external data"
            }
        } else {
            $result.Warnings += "External data directory not found: $EXTERNAL_DATA_PATH"
        }
    } else {
        $result.Warnings += "External drive not found: $EXTERNAL_DRIVE"
    }
    
    # Check if internal configuration is available
    if (Test-Path $CONFIG_INTERNAL) {
        $result.CanSwitchToInternal = $true
    } else {
        $result.Warnings += "Internal configuration file not found: $CONFIG_INTERNAL"
    }
    
    # Determine what to switch TO based on what's currently running
    if ($currentStatus.IsRunning) {
        if ($currentStatus.ConfigType -eq "External") {
            # Currently running external, suggest switching to internal
            if ($result.CanSwitchToInternal) {
                $result.ConfigType = "INTERNAL"
                $result.ConfigFile = $CONFIG_INTERNAL
            } else {
                $result.ConfigType = "EXTERNAL"  # Can't switch, stay external
                $result.ConfigFile = $CONFIG_EXTERNAL
                $result.Warnings += "Cannot switch to internal - configuration not available"
            }
        } else {
            # Currently running internal, suggest switching to external
            if ($result.CanSwitchToExternal) {
                $result.ConfigType = "EXTERNAL"
                $result.ConfigFile = $CONFIG_EXTERNAL
                $result.DataPath = $EXTERNAL_DATA_PATH
            } else {
                $result.ConfigType = "INTERNAL"  # Can't switch, stay internal
                $result.ConfigFile = $CONFIG_INTERNAL
                $result.Warnings += "Cannot switch to external - external drive or data not available"
            }
        }
    } else {
        # MariaDB is not running, prefer external if available, otherwise internal
        if ($result.CanSwitchToExternal) {
            $result.ConfigType = "EXTERNAL"
            $result.ConfigFile = $CONFIG_EXTERNAL
            $result.DataPath = $EXTERNAL_DATA_PATH
        } elseif ($result.CanSwitchToInternal) {
            $result.ConfigType = "INTERNAL"
            $result.ConfigFile = $CONFIG_INTERNAL
        } else {
            $result.Warnings += "No valid configuration available"
        }
    }
    
    # Set data path for internal config if not already set
    if ($result.ConfigType -eq "INTERNAL" -and [string]::IsNullOrEmpty($result.DataPath)) {
        if (Test-Path $CONFIG_INTERNAL) {
            $content = Get-Content $CONFIG_INTERNAL -ErrorAction SilentlyContinue
            $datadir = ($content | Where-Object { $_ -match "^datadir" }) -replace "datadir\s*=\s*", "" -replace '"', ''
            if ($datadir) {
                $result.DataPath = $datadir.Trim() -replace "/", "\"
            } else {
                $result.DataPath = "C:\Program Files\MariaDB 11.8\data\"  # Default path
            }
        }
    }
    
    return $result
}

function Start-MariaDBWithConfig {
    param($ConfigFile, $ConfigType)
    
    try {
        # Stop any existing processes first
        $existingProcess = Get-Process -Name "mysqld" -ErrorAction SilentlyContinue
        if ($existingProcess) {
            Stop-Process -Name "mysqld" -Force -ErrorAction SilentlyContinue
            Start-Sleep -Seconds 3
        }
        
        # Start MariaDB with the specified config
        $startInfo = New-Object System.Diagnostics.ProcessStartInfo
        $startInfo.FileName = "$MARIADB_BIN\mysqld.exe"
        $startInfo.Arguments = "--defaults-file=`"$ConfigFile`""
        $startInfo.UseShellExecute = $false
        $startInfo.CreateNoWindow = $true
        
        $process = [System.Diagnostics.Process]::Start($startInfo)
        
        # Wait a moment and check if it started
        Start-Sleep -Seconds 2
        $newProcess = Get-Process -Name "mysqld" -ErrorAction SilentlyContinue
        
        if ($newProcess) {
            [System.Windows.Forms.MessageBox]::Show("MariaDB started successfully with $ConfigType configuration.", "Success", [System.Windows.Forms.MessageBoxButtons]::OK, [System.Windows.Forms.MessageBoxIcon]::Information)
            return $true
        } else {
            [System.Windows.Forms.MessageBox]::Show("MariaDB failed to start. Check the configuration and data directory.", "Error", [System.Windows.Forms.MessageBoxButtons]::OK, [System.Windows.Forms.MessageBoxIcon]::Error)
            return $false
        }
    } catch {
        [System.Windows.Forms.MessageBox]::Show("Error starting MariaDB: $($_.Exception.Message)", "Error", [System.Windows.Forms.MessageBoxButtons]::OK, [System.Windows.Forms.MessageBoxIcon]::Error)
        return $false
    }
}

function Switch-MariaDBConfiguration {
    try {
        # Check configuration files first
        $issues = Test-ConfigurationFiles
        if ($issues.Count -gt 0) {
            $message = "Configuration file issues found:`n`n" + ($issues -join "`n")
            [System.Windows.Forms.MessageBox]::Show($message, "Configuration Error", [System.Windows.Forms.MessageBoxButtons]::OK, [System.Windows.Forms.MessageBoxIcon]::Error)
            return
        }
        
        # Get current status and suggested switch configuration
        $currentStatus = Get-MariaDBStatus
        $config = Get-OptimalConfiguration
        
        # Build the message based on current state
        $summaryMessage = ""
        
        if ($currentStatus.IsRunning) {
            $summaryMessage += "Current Status: MariaDB is running with $($currentStatus.ConfigType) configuration`n"
            $summaryMessage += "Current Data Path: $($currentStatus.DataPath)`n`n"
            
            # Check if we can actually switch
            if ($currentStatus.ConfigType -eq $config.ConfigType) {
                if ($currentStatus.ConfigType -eq "External" -and -not $config.CanSwitchToInternal) {
                    $summaryMessage += "Cannot switch to Internal configuration - internal config file not available.`n"
                } elseif ($currentStatus.ConfigType -eq "Internal" -and -not $config.CanSwitchToExternal) {
                    $summaryMessage += "Cannot switch to External configuration - external drive or data not available.`n"
                } else {
                    $summaryMessage += "No configuration change needed - already running the only available configuration.`n"
                }
                
                if ($config.Warnings.Count -gt 0) {
                    $summaryMessage += "`nWarnings:`n" + ($config.Warnings -join "`n")
                }
                
                [System.Windows.Forms.MessageBox]::Show($summaryMessage, "Configuration Status", [System.Windows.Forms.MessageBoxButtons]::OK, [System.Windows.Forms.MessageBoxIcon]::Information)
                return
            } else {
                $summaryMessage += "Available Switch: Switch to $($config.ConfigType) configuration`n"
                $summaryMessage += "Target Data Path: $($config.DataPath)`n`n"
                $summaryMessage += "Do you want to restart MariaDB with the $($config.ConfigType) configuration?"
            }
        } else {
            $summaryMessage += "Current Status: MariaDB is not running`n`n"
            $summaryMessage += "Suggested Start: $($config.ConfigType) configuration`n"
            $summaryMessage += "Data Path: $($config.DataPath)`n`n"
            $summaryMessage += "Do you want to start MariaDB with the $($config.ConfigType) configuration?"
        }
        
        # Show warnings if any
        if ($config.Warnings.Count -gt 0) {
            $summaryMessage += "`n`nWarnings:`n" + ($config.Warnings -join "`n")
        }
        
        # Show available options
        $availableConfigs = @()
        if ($config.CanSwitchToInternal) { $availableConfigs += "Internal" }
        if ($config.CanSwitchToExternal) { $availableConfigs += "External" }
        
        if ($availableConfigs.Count -gt 0) {
            $summaryMessage += "`n`nAvailable configurations: " + ($availableConfigs -join ", ")
        }
        
        $result = [System.Windows.Forms.MessageBox]::Show($summaryMessage, "Switch Configuration", [System.Windows.Forms.MessageBoxButtons]::YesNo, [System.Windows.Forms.MessageBoxIcon]::Question)
        
        if ($result -eq [System.Windows.Forms.DialogResult]::Yes) {
            # Validate data directory exists
            if (-not (Test-Path $config.DataPath)) {
                [System.Windows.Forms.MessageBox]::Show("Data directory does not exist: $($config.DataPath)`n`nPlease check your configuration.", "Error", [System.Windows.Forms.MessageBoxButtons]::OK, [System.Windows.Forms.MessageBoxIcon]::Error)
                return
            }
            
            # Check for critical files if switching to external
            if ($config.ConfigType -eq "EXTERNAL") {
                if (-not (Test-Path "$($config.DataPath)\mysql")) {
                    [System.Windows.Forms.MessageBox]::Show("Critical MySQL system database missing in external data.`nPath: $($config.DataPath)\mysql`n`nThis will prevent MariaDB from starting.", "Critical Error", [System.Windows.Forms.MessageBoxButtons]::OK, [System.Windows.Forms.MessageBoxIcon]::Error)
                    return
                }
            }
            
            # Start MariaDB with new configuration
            $success = Start-MariaDBWithConfig -ConfigFile $config.ConfigFile -ConfigType $config.ConfigType
            
            if ($success) {
                # Update tray icon after successful switch
                Start-Sleep -Seconds 1
                Update-TrayIcon
            }
        }
    } catch {
        [System.Windows.Forms.MessageBox]::Show("Error in Switch-MariaDBConfiguration: $($_.Exception.Message)", "Error", [System.Windows.Forms.MessageBoxButtons]::OK, [System.Windows.Forms.MessageBoxIcon]::Error)
    }
}

# Create NotifyIcon
$notifyIcon = New-Object System.Windows.Forms.NotifyIcon
$notifyIcon.Visible = $true

# Create icons (using system icons)
$runningIcon = [System.Drawing.SystemIcons]::Information
$stoppedIcon = [System.Drawing.SystemIcons]::Warning

# Context menu
$contextMenu = New-Object System.Windows.Forms.ContextMenuStrip
$showStatusItem = New-Object System.Windows.Forms.ToolStripMenuItem("Show Status")
$refreshItem = New-Object System.Windows.Forms.ToolStripMenuItem("Refresh")
$switchConfigItem = New-Object System.Windows.Forms.ToolStripMenuItem("Switch Configuration")
$stopItem = New-Object System.Windows.Forms.ToolStripMenuItem("Stop MariaDB")
$separatorItem1 = New-Object System.Windows.Forms.ToolStripSeparator
$separatorItem2 = New-Object System.Windows.Forms.ToolStripSeparator
$exitItem = New-Object System.Windows.Forms.ToolStripMenuItem("Exit")

$contextMenu.Items.Add($showStatusItem)
$contextMenu.Items.Add($refreshItem)
$contextMenu.Items.Add($separatorItem1)
$contextMenu.Items.Add($switchConfigItem)
$contextMenu.Items.Add($stopItem)
$contextMenu.Items.Add($separatorItem2)
$contextMenu.Items.Add($exitItem)

$notifyIcon.ContextMenuStrip = $contextMenu

function Update-TrayIcon {
    try {
        $status = Get-MariaDBStatus
        
        if ($status.IsRunning) {
            $notifyIcon.Icon = $runningIcon
            $notifyIcon.Text = "MariaDB Running ($($status.ConfigType))"
            $stopItem.Enabled = $true
            $switchConfigItem.Enabled = $true
            
            # Show balloon tip on first detection (optional)
            if (-not $script:MariaDBWasRunning) {
                $notifyIcon.BalloonTipTitle = "MariaDB Started"
                $notifyIcon.BalloonTipText = "Configuration: $($status.ConfigType)"
                $notifyIcon.BalloonTipIcon = [System.Windows.Forms.ToolTipIcon]::Info
                $notifyIcon.ShowBalloonTip(3000)
                $script:MariaDBWasRunning = $true
            }
        } else {
            $notifyIcon.Icon = $stoppedIcon
            $notifyIcon.Text = "MariaDB Stopped"
            $stopItem.Enabled = $false
            $switchConfigItem.Enabled = $true
            $script:MariaDBWasRunning = $false
        }
    } catch {
        $notifyIcon.Text = "MariaDB Monitor - Error"
        Write-Host "Error updating tray icon: $($_.Exception.Message)"
    }
}

# Event handlers with improved error handling
$showStatusItem.Add_Click({
    try {
        $status = Get-MariaDBStatus
        $message = if ($status.IsRunning) {
            "MariaDB Status: Running`nConfiguration: $($status.ConfigType)`nData Path: $($status.DataPath)`nProcess ID: $($status.ProcessId)"
        } else {
            "MariaDB Status: Not Running"
        }
        [System.Windows.Forms.MessageBox]::Show($message, "MariaDB Status", [System.Windows.Forms.MessageBoxButtons]::OK, [System.Windows.Forms.MessageBoxIcon]::Information)
    } catch {
        [System.Windows.Forms.MessageBox]::Show("Error retrieving status: $($_.Exception.Message)", "Error", [System.Windows.Forms.MessageBoxButtons]::OK, [System.Windows.Forms.MessageBoxIcon]::Error)
    }
})

$refreshItem.Add_Click({ 
    try {
        Update-TrayIcon 
        $notifyIcon.BalloonTipTitle = "Status Refreshed"
        $notifyIcon.BalloonTipText = "MariaDB status updated"
        $notifyIcon.BalloonTipIcon = [System.Windows.Forms.ToolTipIcon]::Info
        $notifyIcon.ShowBalloonTip(2000)
    } catch {
        Write-Host "Error in refresh: $($_.Exception.Message)"
    }
})

$switchConfigItem.Add_Click({
    try {
        Switch-MariaDBConfiguration
    } catch {
        [System.Windows.Forms.MessageBox]::Show("Error in configuration switch: $($_.Exception.Message)", "Error", [System.Windows.Forms.MessageBoxButtons]::OK, [System.Windows.Forms.MessageBoxIcon]::Error)
    }
})

$stopItem.Add_Click({ 
    try {
        $result = [System.Windows.Forms.MessageBox]::Show("Are you sure you want to stop MariaDB?", "Confirm Stop", [System.Windows.Forms.MessageBoxButtons]::YesNo, [System.Windows.Forms.MessageBoxIcon]::Question)
        if ($result -eq [System.Windows.Forms.DialogResult]::Yes) {
            Stop-MariaDBService
            Start-Sleep -Seconds 1
            Update-TrayIcon
        }
    } catch {
        [System.Windows.Forms.MessageBox]::Show("Error stopping service: $($_.Exception.Message)", "Error", [System.Windows.Forms.MessageBoxButtons]::OK, [System.Windows.Forms.MessageBoxIcon]::Error)
    }
})

$exitItem.Add_Click({ 
    try {
        $notifyIcon.Visible = $false
        $notifyIcon.Dispose()
        [System.Windows.Forms.Application]::Exit()
    } catch {
        # Force exit even if cleanup fails
        [Environment]::Exit(0)
    }
})

# Double-click event to show configuration switcher
$notifyIcon.Add_DoubleClick({
    try {
        Switch-MariaDBConfiguration
    } catch {
        Write-Host "Error in double-click handler: $($_.Exception.Message)"
    }
})

# Timer for periodic updates - FIXED: Added better error handling
$timer = New-Object System.Windows.Forms.Timer
$timer.Interval = 10000  # 10 seconds
$timer.Add_Tick({ 
    try {
        Update-TrayIcon
    } catch {
        # Silently log timer errors to prevent crashes
        Write-Host "Timer error: $($_.Exception.Message)"
    }
})
$timer.Start()

# Initialize tracking variable
$script:MariaDBWasRunning = $false

# Initial update
try {
    Update-TrayIcon
} catch {
    Write-Host "Error in initial update: $($_.Exception.Message)"
}

# Show initial balloon tip
try {
    $notifyIcon.BalloonTipTitle = "MariaDB Monitor"
    $notifyIcon.BalloonTipText = "Monitor started - Right-click for options, Double-click to switch config"
    $notifyIcon.BalloonTipIcon = [System.Windows.Forms.ToolTipIcon]::Info
    $notifyIcon.ShowBalloonTip(3000)
} catch {
    Write-Host "Error showing initial balloon tip: $($_.Exception.Message)"
}

# Keep application running with improved error handling
try {
    [System.Windows.Forms.Application]::Run()
} catch {
    Write-Host "Application error: $($_.Exception.Message)"
} finally {
    # Cleanup
    try {
        if ($timer) { 
            $timer.Stop()
            $timer.Dispose() 
        }
        if ($notifyIcon) { 
            $notifyIcon.Visible = $false
            $notifyIcon.Dispose() 
        }
    } catch {
        # Ignore cleanup errors
    }
}