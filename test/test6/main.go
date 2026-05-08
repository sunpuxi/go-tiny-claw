package main

import "fmt"

func main() {
	matrix := [][]int{{1, 4, 7, 11, 15}, {2, 5, 8, 12, 19}, {3, 6, 9, 16, 22}, {10, 13, 14, 17, 24}}
	fmt.Println(searchMatrix(matrix, 18))
}

func searchMatrix(matrix [][]int, target int) bool {
	// 第一步：先从左往右找元素，如果没有找到target，找到最后一个小于target 的元素 A
	// 第二步：从元素A开始往下找，如果没有找到target，判断当前元素A这一列的数据之中是否存在大于target的元素，如果不存在，那么证明这个target不存在这个矩阵中
	//         如果当前这一列存在大于target的元素，那么找到这一列中第一个大于target的元素B
	// 第三步：从元素B开始向左遍历，如果没有找到target，那么就向左遍历的过程中找到第一个小于target的元素C
	// 后面就重复第二和第三步骤
	// 在二、三步骤中，如果按照遍历的顺序走到了矩阵的边界处，还没有找到符合要求的元素，那么就认为target不存在与矩阵中
	i, j := 0, 0

	if matrix[i][j] == target {
		return true
	}

	m, n := len(matrix), len(matrix[0])

	// 第一步
	APosition := make([]int, 2)
	APosition[0] = i
	for j < n {
		// 找到
		if matrix[i][j] == target {
			return true
		}

		// 找到最后一个元素还是没有找到满足条件的A，则取最后一个元素
		if j == n-1 && matrix[i][j] < target {
			APosition[1] = j
			break
		}

		// 正常情况
		if matrix[i][j] < target && matrix[i][j+1] > target {
			APosition[1] = j
			break
		}

		j++
	}

	for matrix[i][j] != target {
		// 第二步，从A开始，向下找B
		BPosition := make([]int, 2)
		BPosition[1] = j
		for i < m {
			// 找到target
			if matrix[i][j] == target {
				return true
			}

			// 最后一个元素仍然不满足要求，则不存在target
			if i == m-1 && matrix[i][j] < target {
				return false
			}

			// 满足条件的数据
			if matrix[i][j] > target {
				BPosition[0] = i
				break
			}

			i++
		}

		// 第三步，从B开始，往左找C
		CPosition := make([]int, 2)
		CPosition[0] = i
		for j >= 0 {
			// 找到
			if matrix[i][j] == target {
				return true
			}

			// 没有满足条件的C
			if j == 0 && matrix[i][j] > target {
				return false
			}

			// 满足条件的数据
			if matrix[i][j] < target {
				CPosition[1] = j
				break
			}

			j--
		}
	}

	return false
}
