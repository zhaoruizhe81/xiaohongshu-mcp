package main

import (
	"os"
	"strconv"
)

// ReadCommentsConfig 评论区阅读行为参数（可经环境变量覆盖，.env 亦生效）。
type ReadCommentsConfig struct {
	OpenProb                             float64 // 看笔记后翻评论的概率 0~1
	OpenScreensMin, OpenScreensMax       int     // 翻看屏数范围
	CommentProb                          float64 // 评论前翻评论的概率 0~1
	CommentScreensMin, CommentScreensMax int     // 评论前翻看屏数范围
	DwellMin, DwellMax                   int     // 每屏停留秒数范围
}

func defaultReadCommentsConfig() ReadCommentsConfig {
	return ReadCommentsConfig{
		OpenProb: 0.7, OpenScreensMin: 2, OpenScreensMax: 5,
		CommentProb: 0.6, CommentScreensMin: 1, CommentScreensMax: 3,
		DwellMin: 1, DwellMax: 4,
	}
}

// loadReadCommentsConfig 读取环境变量覆盖默认值（无则用默认）。
func loadReadCommentsConfig() ReadCommentsConfig {
	c := defaultReadCommentsConfig()
	c.OpenProb = envFloat("XHS_CR_OPEN_PROB", c.OpenProb)
	c.OpenScreensMin = envInt("XHS_CR_OPEN_SCREENS_MIN", c.OpenScreensMin)
	c.OpenScreensMax = envInt("XHS_CR_OPEN_SCREENS_MAX", c.OpenScreensMax)
	c.CommentProb = envFloat("XHS_CR_COMMENT_PROB", c.CommentProb)
	c.CommentScreensMin = envInt("XHS_CR_COMMENT_SCREENS_MIN", c.CommentScreensMin)
	c.CommentScreensMax = envInt("XHS_CR_COMMENT_SCREENS_MAX", c.CommentScreensMax)
	c.DwellMin = envInt("XHS_CR_DWELL_MIN", c.DwellMin)
	c.DwellMax = envInt("XHS_CR_DWELL_MAX", c.DwellMax)

	// 修正非法值
	if c.OpenProb < 0 {
		c.OpenProb = 0
	}
	if c.OpenProb > 1 {
		c.OpenProb = 1
	}
	if c.CommentProb < 0 {
		c.CommentProb = 0
	}
	if c.CommentProb > 1 {
		c.CommentProb = 1
	}
	if c.OpenScreensMin < 1 {
		c.OpenScreensMin = 1
	}
	if c.OpenScreensMax < c.OpenScreensMin {
		c.OpenScreensMax = c.OpenScreensMin
	}
	if c.CommentScreensMin < 1 {
		c.CommentScreensMin = 1
	}
	if c.CommentScreensMax < c.CommentScreensMin {
		c.CommentScreensMax = c.CommentScreensMin
	}
	if c.DwellMin < 0 {
		c.DwellMin = 0
	}
	if c.DwellMax < c.DwellMin {
		c.DwellMax = c.DwellMin
	}
	return c
}

func envFloat(k string, d float64) float64 {
	if v := os.Getenv(k); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return d
}

func envInt(k string, d int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return d
}
