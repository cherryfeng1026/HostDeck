package collector

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

func ParseMemInfo(raw string) (float64, error) {
	var totalKB, availableKB float64
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		switch strings.TrimSuffix(fields[0], ":") {
		case "MemTotal":
			value, err := strconv.ParseFloat(fields[1], 64)
			if err != nil {
				return 0, err
			}
			totalKB = value
		case "MemAvailable":
			value, err := strconv.ParseFloat(fields[1], 64)
			if err != nil {
				return 0, err
			}
			availableKB = value
		}
	}

	if totalKB == 0 {
		return 0, errors.New("未找到内存总量信息")
	}

	usedKB := totalKB - availableKB
	return usedKB / totalKB * 100, nil
}

func ParseDF(raw string) (float64, error) {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) < 2 {
		return 0, errors.New("磁盘信息输出缺少数据行")
	}

	fields := strings.Fields(lines[1])
	if len(fields) < 5 {
		return 0, errors.New("磁盘信息输出缺少容量字段")
	}

	return strconv.ParseFloat(strings.TrimSuffix(fields[4], "%"), 64)
}

func ParseLoadAvg(raw string) (float64, float64, float64, error) {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) < 3 {
		return 0, 0, 0, errors.New("系统负载输出不完整")
	}

	load1, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, 0, 0, err
	}
	load5, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0, 0, 0, err
	}
	load15, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return 0, 0, 0, err
	}

	return load1, load5, load15, nil
}

func ParseOSRelease(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			return strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
		}
	}
	return strings.TrimSpace(raw)
}

type CPUStatSample struct {
	Idle  uint64
	Total uint64
}

func ParseCPUStat(raw string) (CPUStatSample, error) {
	for _, line := range strings.Split(raw, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 5 || fields[0] != "cpu" {
			continue
		}

		values := make([]uint64, 0, len(fields)-1)
		for _, field := range fields[1:] {
			value, err := strconv.ParseUint(field, 10, 64)
			if err != nil {
				return CPUStatSample{}, fmt.Errorf("解析 CPU 统计失败: %w", err)
			}
			values = append(values, value)
		}

		var total uint64
		for _, value := range values {
			total += value
		}

		idle := values[3]
		if len(values) > 4 {
			idle += values[4]
		}

		return CPUStatSample{Idle: idle, Total: total}, nil
	}

	return CPUStatSample{}, errors.New("未找到 CPU 汇总统计")
}

func CalculateCPUUsage(before, after CPUStatSample) (float64, error) {
	if after.Total < before.Total || after.Idle < before.Idle {
		return 0, errors.New("CPU 统计回退")
	}

	totalDelta := after.Total - before.Total
	idleDelta := after.Idle - before.Idle
	if totalDelta == 0 {
		return 0, errors.New("CPU 统计间隔无变化")
	}

	usage := (1 - float64(idleDelta)/float64(totalDelta)) * 100
	return math.Max(0, math.Min(100, usage)), nil
}

func ParseUptimeSeconds(raw string) (int64, error) {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) == 0 {
		return 0, errors.New("系统运行时长输出为空")
	}

	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("解析运行时长失败: %w", err)
	}
	return int64(value), nil
}
