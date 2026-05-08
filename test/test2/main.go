package main

import "fmt"

// 正方形的矩阵：长宽 N，从 0,0 开始，螺旋遍历这个矩阵
// 1 2 3 10
// 4 5 6 11
// 7 8 9 12
// 13 14 15 16

// 1  2  3  4
// 5  6  7  8
// 9 10 11 12
func main() {
	//data := [][]int{{1, 2, 3, 10}, {4, 5, 6, 11}, {7, 8, 9, 12}, {13, 14, 15, 16}}
	//data := [][]int{{1, 2, 3}, {4, 5, 6}, {7, 8, 9}}
	data := [][]int{{1, 2, 3, 4}, {5, 6, 7, 8}, {9, 10, 11, 12}}
	fmt.Print(spiralOrder(data))
}

func spiralOrder(data [][]int) []int {
	// 四条边界收缩的办法
	res := make([]int, 0)

	top, bottom := 0, len(data)-1
	left, right := 0, len(data[0])-1

	for left <= right && top <= bottom {
		// 收集上面这条边的数据
		for i := left; i <= right; i++ {
			res = append(res, data[top][i])
		}
		top++
		if top > bottom {
			break
		}

		// 收集右边这条边
		for i := top; i <= bottom; i++ {
			res = append(res, data[i][right])
		}
		right--
		if right < left {
			break
		}

		// 收集下面这条边
		for i := right; i >= left; i-- {
			res = append(res, data[bottom][i])
		}
		bottom--
		if bottom < top {
			break
		}

		// 收集左边这条边
		for i := bottom; i >= top; i-- {
			res = append(res, data[i][left])
		}
		left++
	}

	return res
}

// 限制条件改为：输入的矩形是一个 m * n 的
func spiralOrder1(data [][]int) []int {
	// 返回结果
	res := make([]int, 0)

	// 总共的矩形的层数，由较小的那一条变决定
	m := len(data)
	n := len(data[0])
	minimum := min(m, n)

	// 有余数还需要加一，最内层不算一个完整的矩形
	layerCount := 0
	if minimum%2 == 0 {
		layerCount = minimum / 2
	} else {
		layerCount = minimum/2 + 1
	}

	// 遍历每一层的结果
	for i := 0; i < layerCount; i++ {
		// 获取当前开始遍历的下标
		startX := i
		startY := i

		// 计算新的矩形的宽度和高度
		newM := m - i*2 // 1
		newN := n - i*2 // 2
		if newM == 0 || newN == 0 {
			break
		}

		// 开始遍历，先遍历上面这条边，总共需要添加的元素个数为：newN，那么startY的截止坐标就是 startY + newN - 1
		for startY <= i+newN-1 {
			res = append(res, data[startX][startY])
			startY++
		}
		startY-- // 恢复至矩形框中的最后一列的位置

		// 遍历右边这条边，总共需要添加的元素的个数为：newM - 1，上面已经加了第一列最后一个元素，当前需要跳过那个元素
		// startX 的取值范围：[i+1,i+newM-1]
		startX = i + 1
		for startX <= i+newM-1 {
			res = append(res, data[startX][startY])
			startX++
		}
		startX-- // 恢复至矩形框中最后一行的位置

		// 遍历下面这条边，需要添加的元素的个数为：newN - 1
		// startY 的取值范围是：[i,i+newN-2]
		startY--
		for startY >= i && newM > 1 {
			res = append(res, data[startX][startY])
			startY--
		}
		startY++ // 恢复至矩形框中的第一列

		// 遍历左边这条边，需要添加的元素个数为：newM - 2
		// startX 的取值范围是：[i+1,i+newM-2]
		startX--
		for startX >= i+1 && newN > 1 {
			res = append(res, data[startX][startY])
			startX--
		}
	}

	return res
}

func OrderSlice(data [][]int) []int {
	// 结果
	res := make([]int, 0)

	// 总的每一层矩阵的数量的计算公式：num := len(data) % 2 (余数不等于0，则加一)
	num := 0
	if len(data)%2 == 0 {
		num = len(data) / 2
	} else {
		num = len(data)/2 + 1
	}

	// 按照层级依次遍历每个矩阵最外层的数据
	for i := 1; i <= num; i++ {
		// 计算开始遍历的矩阵的下标
		startX := i - 1
		startY := i - 1

		// 这一层矩阵的长度
		sizeOfSlice := len(data) - (i-1)*2

		// 如果内层矩阵只剩下一个元素，直接返回结果即可
		if sizeOfSlice == 1 {
			res = append(res, data[startX][startY])
			break
		}

		// 开始遍历 矩阵上面这条边，从左到右
		for startY < sizeOfSlice {
			res = append(res, data[startX][startY])
			startY++
		}
		// 循环结束之后的 startY 已经到下一个矩阵的坐标，需要减一，回到当前矩阵
		startY--

		// 遍历右边这条边，从上到下
		startX++ // 防止重复添加最后一次遍历的结果
		for startX < sizeOfSlice {
			res = append(res, data[startX][startY])
			startX++
		}
		startX--

		// 开始遍历矩阵的下边，从右到左
		startY--
		for startY >= i-1 {
			res = append(res, data[startX][startY])
			startY--
		}
		startY++

		// 遍历矩阵左边的边，从下往上，最后一次遍历的时候，
		// 最后一个元素已经在第一条边的遍历过程中添加了，所以不取等号
		startX--
		for startX > i-1 {
			res = append(res, data[startX][startY])
			startX--
		}

	}

	return res
}
