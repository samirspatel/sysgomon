package main

import (
	"fmt"
	"log"
	"sort"
	"time"

	ui "github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"github.com/shirou/gopsutil/v3/process"
)

// ProcessInfo represents a process with its resource usage
type ProcessInfo struct {
	PID     int32
	Name    string
	CPU     float64
	Memory  float64
	Command string
}

// ProcessList widget for displaying top processes
type ProcessList struct {
	*widgets.Table
	Processes []ProcessInfo
}

func createProcessList(x, y, width, height int) *ProcessList {
	pl := &ProcessList{
		Table: widgets.NewTable(),
	}
	pl.Title = "Top Processes"
	pl.Border = true
	pl.SetRect(x, y, width, height)
	pl.Rows = [][]string{
		{"Name", "CPU%", "Mem%", "Command"},
	}
	pl.TextStyle = ui.NewStyle(ui.ColorWhite)
	pl.updateColumnWidths(width)
	return pl
}

func (pl *ProcessList) updateColumnWidths(width int) {
	pl.ColumnWidths = []int{
		int(float64(width) * 0.2), // Name: 20% of width
		int(float64(width) * 0.1), // CPU%: 10% of width
		int(float64(width) * 0.1), // Mem%: 10% of width
		int(float64(width) * 0.6), // Command: 60% of width
	}
}

func (pl *ProcessList) collectProcessInfo() error {
	processes, err := process.Processes()
	if err != nil {
		return err
	}

	pl.Processes = make([]ProcessInfo, 0)
	for _, p := range processes {
		info, err := pl.getProcessInfo(p)
		if err != nil {
			continue
		}
		pl.Processes = append(pl.Processes, info)
	}
	return nil
}

func (pl *ProcessList) getProcessInfo(p *process.Process) (ProcessInfo, error) {
	name, err := p.Name()
	if err != nil {
		return ProcessInfo{}, err
	}

	cpu, err := p.CPUPercent()
	if err != nil {
		return ProcessInfo{}, err
	}

	mem, err := p.MemoryPercent()
	if err != nil {
		return ProcessInfo{}, err
	}

	cmd, err := p.Cmdline()
	if err != nil {
		cmd = name
	}

	return ProcessInfo{
		PID:     p.Pid,
		Name:    name,
		CPU:     cpu,
		Memory:  float64(mem),
		Command: cmd,
	}, nil
}

func (pl *ProcessList) sortProcesses() {
	sort.Slice(pl.Processes, func(i, j int) bool {
		return pl.Processes[i].CPU > pl.Processes[j].CPU
	})
}

func (pl *ProcessList) formatCommand(cmd string, width int) string {
	if len(cmd) > width {
		return cmd[:width-3] + "..."
	}
	return cmd
}

func (pl *ProcessList) update() {
	// Update column widths based on current width
	pl.updateColumnWidths(pl.Block.Rectangle.Dx())

	// Collect and sort process information
	if err := pl.collectProcessInfo(); err != nil {
		pl.Rows = [][]string{{"Error getting processes"}}
		return
	}
	pl.sortProcesses()

	// Update table rows
	rows := make([][]string, 0)
	rows = append(rows, []string{"Name", "CPU%", "Mem%", "Command"})

	// Calculate available width for command column
	availableWidth := pl.Block.Rectangle.Dx() - 2
	commandWidth := int(float64(availableWidth) * 0.6)

	// Add process rows
	for _, p := range pl.Processes {
		rows = append(rows, []string{
			p.Name,
			fmt.Sprintf("%.1f", p.CPU),
			fmt.Sprintf("%.1f", p.Memory),
			pl.formatCommand(p.Command, commandWidth),
		})
	}

	pl.Rows = rows
}

// CPUGauge tracks a CPU gauge with its previous value and target value for smooth transitions
type CPUGauge struct {
	*widgets.Gauge
	CurrentPercent float64 // Current displayed value (for smooth transitions)
	TargetPercent  float64 // Target value to animate towards
}

// NetworkData stores network traffic data for graphing
type NetworkData struct {
	RxData   []float64 // History of received data rates
	TxData   []float64 // History of transmitted data rates
	MaxValue float64   // Maximum value for scaling
}

// DiskData stores disk I/O data for graphing
type DiskData struct {
	ReadData  []float64 // History of read speeds
	WriteData []float64 // History of write speeds
	MaxValue  float64   // Maximum value for scaling
}

func main() {
	if err := ui.Init(); err != nil {
		log.Fatalf("Failed to initialize termui: %v", err)
	}
	defer ui.Close()

	// Set the animation speed (lower = slower transitions)
	animationSpeed := 0.03 // How quickly to transition to target value

	// Get the terminal dimensions
	termWidth, termHeight := ui.TerminalDimensions()

	// Create header with system info
	header := widgets.NewParagraph()
	header.Title = "SysGoMon"
	header.Border = true
	header.SetRect(0, 0, termWidth, 3)
	header.TextStyle.Fg = ui.ColorCyan
	header.TitleStyle.Fg = ui.ColorWhite

	// Create CPU gauges
	cpuTitle, cpuGauges, cpuHeight := createCPUGauges(termWidth)

	// Create Network stats and graph
	netStats := widgets.NewParagraph()
	netStats.Title = "Network Traffic"
	netStats.Border = true
	netStats.SetRect(0, cpuHeight, termWidth, cpuHeight+4)
	netStats.TitleStyle.Fg = ui.ColorWhite

	// Network graph for historical data
	netGraph := widgets.NewPlot()
	netGraph.Title = "Network Traffic History (Mbps)"
	netGraph.Border = true
	netGraph.LineColors[0] = ui.ColorGreen  // RX
	netGraph.LineColors[1] = ui.ColorBlue   // TX
	netGraph.AxesColor = ui.ColorWhite
	netGraph.DrawDirection = widgets.DrawRight  // Draw from left to right
	netGraph.SetRect(0, netStats.Block.Rectangle.Max.Y, termWidth, netStats.Block.Rectangle.Max.Y+9)
	netGraph.TitleStyle.Fg = ui.ColorWhite
	// Use the terminal width to determine how many data points to store
	// This ensures we have enough points to span the entire width
	dataPointCount := termWidth // Use full terminal width to ensure graph touches the right edge
	if dataPointCount < 100 {
		dataPointCount = 100 // Minimum size
	}
	netGraph.Data = make([][]float64, 2)
	netGraph.Data[0] = make([]float64, dataPointCount) // RX data with terminal-width adjusted count
	netGraph.Data[1] = make([]float64, dataPointCount) // TX data with terminal-width adjusted count
	netGraph.PlotType = widgets.LineChart  // Use line chart for better visibility
	netGraph.ShowAxes = false  // Hide the axis numbers
	netGraph.HorizontalScale = 1.0  // Ensure it uses full width
	// Set plot mode to stretch to fill the entire width
	netGraph.AxesColor = ui.ColorClear // Make axes invisible

	// Network traffic history
	netData := NetworkData{
		RxData: make([]float64, dataPointCount),
		TxData: make([]float64, dataPointCount),
		MaxValue: 0.1, // Start with a small non-zero value
	}

	// Create Disk I/O stats and graph
	diskStats := widgets.NewParagraph()
	diskStats.Title = "Disk I/O"
	diskStats.Border = true
	diskStats.SetRect(0, netGraph.Block.Rectangle.Max.Y, termWidth, netGraph.Block.Rectangle.Max.Y+4)
	diskStats.TitleStyle.Fg = ui.ColorWhite

	// Disk I/O graph for historical data
	diskGraph := widgets.NewPlot()
	diskGraph.Title = "Disk I/O History (MB/s)"
	diskGraph.Border = true
	diskGraph.LineColors[0] = ui.ColorGreen  // Read
	diskGraph.LineColors[1] = ui.ColorRed    // Write
	diskGraph.AxesColor = ui.ColorWhite
	diskGraph.DrawDirection = widgets.DrawRight  // Draw from left to right
	diskGraph.SetRect(0, diskStats.Block.Rectangle.Max.Y, termWidth, diskStats.Block.Rectangle.Max.Y+9)
	diskGraph.TitleStyle.Fg = ui.ColorWhite
	diskGraph.Data = make([][]float64, 2)
	diskGraph.Data[0] = make([]float64, dataPointCount) // Read data
	diskGraph.Data[1] = make([]float64, dataPointCount) // Write data
	diskGraph.PlotType = widgets.LineChart  // Use line chart for better visibility
	diskGraph.ShowAxes = false  // Hide the axis numbers
	diskGraph.HorizontalScale = 1.0  // Ensure it uses full width
	diskGraph.AxesColor = ui.ColorClear // Make axes invisible

	// Disk I/O history
	diskData := DiskData{
		ReadData:  make([]float64, dataPointCount),
		WriteData: make([]float64, dataPointCount),
		MaxValue:  0.1, // Start with a small non-zero value
	}

	// Create process list
	processList := createProcessList(0, diskGraph.Block.Rectangle.Max.Y, termWidth, termHeight-1)
	processList.TitleStyle.Fg = ui.ColorWhite

	// Create footer with instructions
	footer := widgets.NewParagraph()
	footer.Border = false
	footer.Text = "[Press q to quit](fg:red)"
	footer.SetRect(0, termHeight-1, termWidth, termHeight)

	// Get initial network stats for baseline
	netIOCounters, err := net.IOCounters(false)
	if err != nil {
		log.Printf("Error getting network stats: %v", err)
	}
	prevNetIOStats := netIOCounters[0]
	lastNetworkUpdate := time.Now()

	// Get initial disk stats for baseline
	diskIOCounters, err := disk.IOCounters()
	if err != nil {
		log.Printf("Error getting disk I/O stats: %v", err)
	}
	prevDiskIOStats := make(map[string]disk.IOCountersStat)
	for name, stat := range diskIOCounters {
		prevDiskIOStats[name] = stat
	}
	lastDiskUpdate := time.Now()

	// Update system info in header
	updateHeader(header)

	// Initial render to set up the screen
	ui.Clear()
	ui.Render(header, cpuTitle)
	for _, gauge := range cpuGauges {
		ui.Render(gauge.Gauge)
	}
	ui.Render(netStats, netGraph, diskStats, diskGraph, processList, footer)

	// Set up event handling
	uiEvents := ui.PollEvents()
	ticker := time.NewTicker(300 * time.Millisecond).C // Update every half second for more responsive display

	// Main event loop
	for {
		select {
		case e := <-uiEvents:
			switch e.ID {
			case "q", "<C-c>":
				return
			case "<Resize>":
				payload := e.Payload.(ui.Resize)
				termWidth, termHeight = payload.Width, payload.Height
				
				header.SetRect(0, 0, termWidth, 3)
				
				// Update CPU gauges position
				cpuTitle, cpuGauges, cpuHeight = createCPUGauges(termWidth)
				
				// Update network stats and graph positions
				netStats.SetRect(0, cpuHeight, termWidth, cpuHeight+4)
				netGraph.SetRect(0, netStats.Block.Rectangle.Max.Y, termWidth, netStats.Block.Rectangle.Max.Y+9)
				
				// Update data point count on resize to match new width
				dataPointCount := termWidth // Use full terminal width
				if dataPointCount < 100 {
					dataPointCount = 100
				}
				// Only recreate data arrays if new width requires more points
				if dataPointCount > len(netData.RxData) {
					newRxData := make([]float64, dataPointCount)
					newTxData := make([]float64, dataPointCount)
					
					// Copy existing data to preserve history
					copy(newRxData[dataPointCount-len(netData.RxData):], netData.RxData)
					copy(newTxData[dataPointCount-len(netData.TxData):], netData.TxData)
					
					netData.RxData = newRxData
					netData.TxData = newTxData
					
					netGraph.Data[0] = netData.RxData
					netGraph.Data[1] = netData.TxData
				}
				
				// Update disk graph data points if needed
				if dataPointCount > len(diskData.ReadData) {
					newReadData := make([]float64, dataPointCount)
					newWriteData := make([]float64, dataPointCount)
					
					// Copy existing data to preserve history
					copy(newReadData[dataPointCount-len(diskData.ReadData):], diskData.ReadData)
					copy(newWriteData[dataPointCount-len(diskData.WriteData):], diskData.WriteData)
					
					diskData.ReadData = newReadData
					diskData.WriteData = newWriteData
					
					diskGraph.Data[0] = diskData.ReadData
					diskGraph.Data[1] = diskData.WriteData
				}
				
				diskStats.SetRect(0, netGraph.Block.Rectangle.Max.Y, termWidth, netGraph.Block.Rectangle.Max.Y+4)
				diskGraph.SetRect(0, diskStats.Block.Rectangle.Max.Y, termWidth, diskStats.Block.Rectangle.Max.Y+9)
				
				// Update process list position
				processList.SetRect(0, diskGraph.Block.Rectangle.Max.Y, termWidth, termHeight-1)
				
				footer.SetRect(0, termHeight-1, termWidth, termHeight)
				
				// Complete redraw is necessary on resize
				ui.Clear()
				ui.Render(header, cpuTitle)
				for _, gauge := range cpuGauges {
					ui.Render(gauge.Gauge)
				}
				ui.Render(netStats, netGraph, diskStats, diskGraph, processList, footer)
			}

		case <-ticker:
			// Update CPU gauges target values
			updateCPUTargets(cpuGauges)
			
			// Animate CPU gauges toward target values
			animateCPUGauges(cpuGauges, animationSpeed)
			
			// Update network information
			now := time.Now()
			if netIOCounters, err := net.IOCounters(false); err == nil {
				duration := now.Sub(lastNetworkUpdate).Seconds()
				rxBytesPerSec := float64(netIOCounters[0].BytesRecv-prevNetIOStats.BytesRecv) / duration
				txBytesPerSec := float64(netIOCounters[0].BytesSent-prevNetIOStats.BytesSent) / duration
				
				rxMbps := rxBytesPerSec * 8 / 1000000 // Convert bytes/sec to Mbps
				txMbps := txBytesPerSec * 8 / 1000000 // Convert bytes/sec to Mbps
				
				// Update network text display
				newText := fmt.Sprintf(
					"[In:  ](fg:green) %8.2f Mbps  [Out: ](fg:blue) %8.2f Mbps  [Total In: ](fg:cyan) %s  [Total Out:](fg:cyan) %s",
					rxMbps, 
					txMbps,
					formatBytes(netIOCounters[0].BytesRecv),
					formatBytes(netIOCounters[0].BytesSent),
				)
				
				// Only update if the text changed
				if newText != netStats.Text {
					netStats.Text = newText
					ui.Render(netStats)
				}
				
				// Shift network history data and add new values
				updateNetworkGraph(&netData, rxMbps, txMbps, netGraph)
				
				prevNetIOStats = netIOCounters[0]
				lastNetworkUpdate = now
			}
			
			// Update disk I/O information
			if diskIOCounters, err := disk.IOCounters(); err == nil {
				duration := now.Sub(lastDiskUpdate).Seconds()
				diskText := ""
				
				// Calculate total read and write speeds across all disks
				var totalReadMBps, totalWriteMBps float64
				for name, stat := range diskIOCounters {
					if prev, ok := prevDiskIOStats[name]; ok {
						readBytesPerSec := float64(stat.ReadBytes-prev.ReadBytes) / duration / 1024 / 1024  // MB/s
						writeBytesPerSec := float64(stat.WriteBytes-prev.WriteBytes) / duration / 1024 / 1024 // MB/s
						
						totalReadMBps += readBytesPerSec
						totalWriteMBps += writeBytesPerSec
						
						if len(diskText) > 0 {
							diskText += "\n"
						}
						diskText += fmt.Sprintf(
							"[%s](fg:yellow) Read: [%.2f MB/s](fg:green) Write: [%.2f MB/s](fg:red)",
							name, readBytesPerSec, writeBytesPerSec,
						)
					}
					prevDiskIOStats[name] = stat
				}
				
				// Only update if the text changed
				if diskText != diskStats.Text {
					diskStats.Text = diskText
					ui.Render(diskStats)
				}
				
				// Update disk I/O graph
				updateDiskGraph(&diskData, totalReadMBps, totalWriteMBps, diskGraph)
				
				lastDiskUpdate = now
			}
			
			// Update process list
			processList.update()
			ui.Render(processList)
		}
	}
}

func createCPUGauges(width int) (*widgets.Paragraph, []CPUGauge, int) {
	// Get number of CPU cores
	cpuCount, err := cpu.Counts(true)
	if err != nil {
		log.Printf("Error getting CPU count: %v", err)
		cpuCount = 1
	}

	// Create title paragraph
	cpuTitle := widgets.NewParagraph()
	cpuTitle.Text = fmt.Sprintf("[CPU Utilization (%d cores)](fg:white,mod:bold)", cpuCount)
	cpuTitle.Border = false
	cpuTitle.SetRect(0, 3, width, 4) // Start at y=3 (after header)
	
	// Create individual gauges for each CPU core
	gauges := make([]CPUGauge, cpuCount+1) // +1 for the average
	
	// First gauge is for average CPU
	gauges[0] = CPUGauge{
		Gauge: widgets.NewGauge(),
		CurrentPercent: 0,
		TargetPercent: 0,
	}
	gauges[0].Gauge.Title = "Avg CPU"
	gauges[0].Gauge.SetRect(0, 4, width, 7) // Start at y=4
	gauges[0].Gauge.BarColor = ui.ColorGreen
	gauges[0].Gauge.BorderStyle.Fg = ui.ColorBlue
	gauges[0].Gauge.TitleStyle.Fg = ui.ColorCyan
	
	// Calculate the width for each column
	columnWidth := width / 2
	
	// Create a gauge for each CPU core
	for i := 0; i < cpuCount; i++ {
		gauges[i+1] = CPUGauge{
			Gauge: widgets.NewGauge(),
			CurrentPercent: 0,
			TargetPercent: 0,
		}
		gauges[i+1].Gauge.Title = fmt.Sprintf("CPU %d", i+1)
		
		// Determine which column this CPU belongs to
		isLeftColumn := i < cpuCount/2
		
		// Calculate x position based on column
		xStart := 0
		if !isLeftColumn {
			xStart = columnWidth
		}
		
		// Calculate y position based on position within column
		yOffset := i
		if !isLeftColumn {
			yOffset = i - cpuCount/2
		}
		
		// Ensure the right column gauges extend to the full width
		xEnd := xStart + columnWidth
		if !isLeftColumn {
			xEnd = width // Make right column extend to full width
		}
		
		gauges[i+1].Gauge.SetRect(xStart, 7+yOffset*3, xEnd, 10+yOffset*3)
		gauges[i+1].Gauge.BarColor = ui.ColorGreen
		gauges[i+1].Gauge.BorderStyle.Fg = ui.ColorBlue
		gauges[i+1].Gauge.TitleStyle.Fg = ui.ColorCyan
	}
	
	// Calculate total height based on the number of rows needed
	rowsPerColumn := (cpuCount + 1) / 2 // Round up for odd number of cores
	totalHeight := 7 + rowsPerColumn*3 // Start from y=7
	
	return cpuTitle, gauges, totalHeight
}

func updateCPUTargets(gauges []CPUGauge) {
	// Get percent of each CPU
	percentages, err := cpu.Percent(0, true)
	if err != nil {
		log.Printf("Error getting CPU percentages: %v", err)
		return
	}

	// Calculate average
	var totalPercent float64
	for _, percent := range percentages {
		totalPercent += percent
	}
	avgPercent := totalPercent / float64(len(percentages))
	
	// Update average gauge target
	gauges[0].TargetPercent = avgPercent
	
	// Update individual CPU gauge targets
	for i, percent := range percentages {
		if i+1 < len(gauges) {
			gauges[i+1].TargetPercent = percent
		}
	}
}

func animateCPUGauges(gauges []CPUGauge, speed float64) {
	// Animate all gauges toward their target values
	for i := range gauges {
		// Calculate the next step in animation
		diff := gauges[i].TargetPercent - gauges[i].CurrentPercent
		
		// If difference is very small, just snap to the target
		if abs(diff) < 0.5 {
			gauges[i].CurrentPercent = gauges[i].TargetPercent
		} else {
			// Otherwise, move a percentage of the way to the target
			gauges[i].CurrentPercent += diff * speed
		}
		
		// Update gauge percent
		intPercent := int(gauges[i].CurrentPercent)
		gauges[i].Gauge.Percent = intPercent
		
		// Update color based on usage
		if intPercent >= 80 {
			gauges[i].Gauge.BarColor = ui.ColorRed
		} else if intPercent >= 50 {
			gauges[i].Gauge.BarColor = ui.ColorYellow
		} else {
			gauges[i].Gauge.BarColor = ui.ColorGreen
		}
		
		// Render just this gauge
		ui.Render(gauges[i].Gauge)
	}
}

func updateNetworkGraph(netData *NetworkData, rxMbps, txMbps float64, graph *widgets.Plot) {
	shiftNetworkData(netData)
	addNetworkData(netData, rxMbps, txMbps)
	updateNetworkMaxValue(netData)
	updateNetworkGraphDisplay(netData, rxMbps, txMbps, graph)
}

func shiftNetworkData(netData *NetworkData) {
	for i := 0; i < len(netData.RxData)-1; i++ {
		netData.RxData[i] = netData.RxData[i+1]
		netData.TxData[i] = netData.TxData[i+1]
	}
}

func addNetworkData(netData *NetworkData, rxMbps, txMbps float64) {
	netData.RxData[len(netData.RxData)-1] = rxMbps
	netData.TxData[len(netData.TxData)-1] = txMbps
}

func updateNetworkMaxValue(netData *NetworkData) {
	currentMax := max(maxInSlice(netData.RxData), maxInSlice(netData.TxData))
	if currentMax > netData.MaxValue {
		netData.MaxValue = netData.MaxValue + (currentMax-netData.MaxValue)*0.3
	} else if currentMax < netData.MaxValue*0.5 && netData.MaxValue > 1.0 {
		netData.MaxValue = netData.MaxValue - (netData.MaxValue-currentMax)*0.05
	}

	if netData.MaxValue < 0.1 {
		netData.MaxValue = 0.1
	}
}

func updateNetworkGraphDisplay(netData *NetworkData, rxMbps, txMbps float64, graph *widgets.Plot) {
	graph.Data[0] = netData.RxData
	graph.Data[1] = netData.TxData
	graph.PlotType = widgets.LineChart
	graph.AxesColor = ui.ColorClear

	graph.DataLabels = []string{
		fmt.Sprintf("In (%.1f Mbps)", rxMbps),
		fmt.Sprintf("Out (%.1f Mbps)", txMbps),
	}

	timeSpan := len(netData.RxData) / 2
	graph.Title = fmt.Sprintf("Network Traffic History (last ~%d seconds) - Max: %.1f Mbps", timeSpan, netData.MaxValue)

	ui.Render(graph)
}

func updateDiskGraph(diskData *DiskData, readMBps, writeMBps float64, graph *widgets.Plot) {
	shiftDiskData(diskData)
	addDiskData(diskData, readMBps, writeMBps)
	updateDiskMaxValue(diskData)
	updateDiskGraphDisplay(diskData, readMBps, writeMBps, graph)
}

func shiftDiskData(diskData *DiskData) {
	for i := 0; i < len(diskData.ReadData)-1; i++ {
		diskData.ReadData[i] = diskData.ReadData[i+1]
		diskData.WriteData[i] = diskData.WriteData[i+1]
	}
}

func addDiskData(diskData *DiskData, readMBps, writeMBps float64) {
	diskData.ReadData[len(diskData.ReadData)-1] = readMBps
	diskData.WriteData[len(diskData.WriteData)-1] = writeMBps
}

func updateDiskMaxValue(diskData *DiskData) {
	currentMax := max(maxInSlice(diskData.ReadData), maxInSlice(diskData.WriteData))
	if currentMax > diskData.MaxValue {
		diskData.MaxValue = diskData.MaxValue + (currentMax-diskData.MaxValue)*0.3
	} else if currentMax < diskData.MaxValue*0.5 && diskData.MaxValue > 1.0 {
		diskData.MaxValue = diskData.MaxValue - (diskData.MaxValue-currentMax)*0.05
	}

	if diskData.MaxValue < 0.1 {
		diskData.MaxValue = 0.1
	}
}

func updateDiskGraphDisplay(diskData *DiskData, readMBps, writeMBps float64, graph *widgets.Plot) {
	graph.Data[0] = diskData.ReadData
	graph.Data[1] = diskData.WriteData
	graph.PlotType = widgets.LineChart
	graph.AxesColor = ui.ColorClear

	graph.DataLabels = []string{
		fmt.Sprintf("Read (%.2f MB/s)", readMBps),
		fmt.Sprintf("Write (%.2f MB/s)", writeMBps),
	}

	timeSpan := len(diskData.ReadData) / 2
	graph.Title = fmt.Sprintf("Disk I/O History (last ~%d seconds) - Max: %.2f MB/s", timeSpan, diskData.MaxValue)

	ui.Render(graph)
}

// Helper functions 
func maxInSlice(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	
	max := values[0]
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	return max
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func updateHeader(p *widgets.Paragraph) {
	hostInfo, err := host.Info()
	if err != nil {
		log.Printf("Error getting host info: %v", err)
		p.Text = "Error getting system information"
		return
	}
	
	memInfo, err := mem.VirtualMemory()
	if err != nil {
		log.Printf("Error getting memory info: %v", err)
	}
	
	cpuCount, err := cpu.Counts(true)
	if err != nil {
		log.Printf("Error getting CPU count: %v", err)
	}

	// Get disk usage information
	diskInfo, err := disk.Usage("/")
	if err != nil {
		log.Printf("Error getting disk info: %v", err)
	}
	
	p.Text = fmt.Sprintf(
		"[Host: %s](fg:cyan) | [OS: %s %s](fg:yellow) | [%d cores](fg:green) | [RAM: %s / %s (%.1f%%)](fg:magenta) | [Disk: %s free / %s total (%.1f%% free)](fg:red)",
		hostInfo.Hostname,
		hostInfo.Platform,
		hostInfo.PlatformVersion,
		cpuCount,
		formatBytes(memInfo.Used),
		formatBytes(memInfo.Total),
		memInfo.UsedPercent,
		formatBytes(diskInfo.Free),
		formatBytes(diskInfo.Total),
		100-diskInfo.UsedPercent,
	)
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
} 