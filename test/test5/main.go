package main

import "fmt"

func main() {
	// data := [][]int{{1, 2, 3}, {4, 5, 6}, {7, 8, 9}}
	data := [][]int{{1, 2, 3, 4}, {5, 6, 7, 8}, {9, 10, 11, 12}, {13, 14, 15, 16}}
	rotate(data)
}

func rotate(matrix [][]int) {
	// 假设：右下角的元素为A，右上角的元素为 B，左上角的元素为C，左下角的元素为D
	// 那么交换顺序就是 A <-> B,C <-> D,D <-> B
	// 总共需要交换的层数,边长是奇数的时候，最内层的元素一定是一个方块
	n := len(matrix)
	layers := (n + 1) / 2

	// 依次处理每一层
	for i := 1; i <= layers; i++ {
		// 分别计算 A、B、C、D 的坐标
		APosition := []int{(n - 1) - (i - 1), i - 1}
		BPosition := []int{i - 1, i - 1}
		CPosition := []int{0 + (i - 1), (n - 1) - (i - 1)}
		DPosition := []int{(n - 1) - (i - 1), (n - 1) - (i - 1)}

		length := APosition[0] - BPosition[0] + 1
		for j := 0; j < length-1; j++ {
			// 交换A，B
			temp := matrix[APosition[0]][APosition[1]]
			matrix[APosition[0]][APosition[1]] = matrix[BPosition[0]][BPosition[1]]
			matrix[BPosition[0]][BPosition[1]] = temp

			// 交换C，D
			temp2 := matrix[CPosition[0]][CPosition[1]]
			matrix[CPosition[0]][CPosition[1]] = matrix[DPosition[0]][DPosition[1]]
			matrix[DPosition[0]][DPosition[1]] = temp2

			// 交换A,C
			temp3 := matrix[APosition[0]][APosition[1]]
			matrix[APosition[0]][APosition[1]] = matrix[CPosition[0]][CPosition[1]]
			matrix[CPosition[0]][CPosition[1]] = temp3

			// A 往上移一位
			APosition = []int{APosition[0] - 1, APosition[1]}
			// B 往右移一位
			BPosition = []int{BPosition[0], BPosition[1] + 1}
			// C 往下移一位
			CPosition = []int{CPosition[0] + 1, CPosition[1]}
			// D 往左移一位
			DPosition = []int{DPosition[0], DPosition[1] - 1}
		}
	}

	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			fmt.Printf("%d ", matrix[i][j])
		}
		fmt.Println()
	}
}
