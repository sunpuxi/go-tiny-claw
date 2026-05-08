package main

import (
	"fmt"
	"sort"
)

func main() {
	testdata := [][]int{{1, 3}, {2, 6}, {8, 10}, {15, 18}}
	// testdata := [][]int{{1, 4}, {4, 5}}
	fmt.Println(merge(testdata))
}

func merge(intervals [][]int) [][]int {
	// 返回结果
	res := make([][]int, 0)

	// 排序
	sort.Slice(intervals, func(a, b int) bool {
		return intervals[a][0] < intervals[b][0]
	})

	// 依次合并区间
	idxOfsort := 0
	for idxOfsort < len(intervals) {
		idxOfNext := idxOfsort + 1

		// 超过切片的长度则直接返回
		if idxOfNext >= len(intervals) {
			res = append(res, intervals[idxOfsort])
			break
		}

		// 依次合并区间
		newArr := intervals[idxOfsort]
		j := idxOfNext
		for j < len(intervals) {
			if intervals[j][0] <= newArr[1] {
				newArr[1] = max(newArr[1], intervals[j][1])
				j++
				idxOfsort = j
				if idxOfsort >= len(intervals) {
					res = append(res, newArr)
					break
				}
			} else {
				res = append(res, newArr)
				newArr = intervals[j]
				idxOfsort = j
				break
			}
		}
	}

	return res
}
