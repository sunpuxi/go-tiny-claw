package main

import "fmt"

// 0 1 2 0
// 3 4 5 2
// 1 3 1 5
func main() {
	//data := [][]int{{1, 1, 1}, {1, 0, 1}, {1, 1, 1}}
	data := [][]int{{0, 1, 2, 0}, {3, 4, 5, 2}, {1, 3, 1, 5}}
	setZeroes(data)
}

func setZeroes(matrix [][]int) {
	// 使用矩阵的第一行和第一列来代替解法一中的 map
	lineZero := false
	columnZero := false

	m := len(matrix)
	n := len(matrix[0])

	for j := 0; j < n; j++ {
		if matrix[0][j] == 0 {
			lineZero = true
		}
	}

	for i := 0; i < m; i++ {
		if matrix[i][0] == 0 {
			columnZero = true
		}
	}

	for i := 1; i < m; i++ {
		for j := 1; j < n; j++ {
			if matrix[i][j] == 0 {
				matrix[i][0] = 0
				matrix[0][j] = 0
			}
		}
	}

	for i := 1; i < m; i++ {
		for j := 1; j < n; j++ {
			if matrix[0][j] == 0 || matrix[i][0] == 0 {
				matrix[i][j] = 0
			}
		}
	}

	// 判断是否需要将第一行和第一列的元素置为0
	if lineZero {
		for j := 0; j < n; j++ {
			matrix[0][j] = 0
		}
	}

	// 判断是否需要将第一列置为0
	if columnZero {
		for i := 0; i < m; i++ {
			matrix[i][0] = 0
		}
	}

	// 输出
	for i := 0; i < m; i++ {
		for j := 0; j < n; j++ {
			fmt.Print(matrix[i][j], " ")
		}
		fmt.Println()
	}

}

func setZeroes1(matrix [][]int) {
	// 解法一：先标记出所有0所在的行坐标，和列坐标
	lineMap := make(map[int]bool)
	columnMap := make(map[int]bool)

	m := len(matrix)
	n := len(matrix[0])

	for i := 0; i < m; i++ {
		for j := 0; j < n; j++ {
			if matrix[i][j] == 0 {
				lineMap[i] = true
				columnMap[j] = true
			}
		}
	}

	// 先将所有有0 的行全部标记为0
	for key, _ := range lineMap {
		for j := 0; j < n; j++ {
			matrix[key][j] = 0
		}
	}

	// 将所有的列标记为 0
	for key, _ := range columnMap {
		for j := 0; j < m; j++ {
			matrix[j][key] = 0
		}
	}

	// 输出
	for i := 0; i < m; i++ {
		for j := 0; j < n; j++ {
			fmt.Print(matrix[i][j], " ")
		}
		fmt.Println()
	}

}

func setZeroes2(matrix [][]int) {
	// 解法一：先标记出所有0所在的行坐标，和列坐标
	lineMap := make(map[int]bool)
	columnMap := make(map[int]bool)

	m := len(matrix)
	n := len(matrix[0])

	for i := 0; i < m; i++ {
		for j := 0; j < n; j++ {
			if matrix[i][j] == 0 {
				lineMap[i] = true
				columnMap[j] = true
			}
		}
	}

	// 先将所有有0 的行全部标记为0
	for key, _ := range lineMap {
		for j := 0; j < n; j++ {
			matrix[key][j] = 0
		}
	}

	// 将所有的列标记为 0
	for key, _ := range columnMap {
		for j := 0; j < m; j++ {
			matrix[j][key] = 0
		}
	}

}
