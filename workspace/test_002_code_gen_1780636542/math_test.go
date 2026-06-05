package math

import "testing"

func TestMultiply(t *testing.T) {
	tests := []struct {
		name     string
		a        int
		b        int
		expected int
	}{
		{"正数相乘", 2, 3, 6},
		{"负数相乘", -2, -3, 6},
		{"正负数相乘", 2, -3, -6},
		{"负正数相乘", -2, 3, -6},
		{"零相乘", 0, 5, 0},
		{"与零相乘", 5, 0, 0},
		{"零与零相乘", 0, 0, 0},
		{"大数相乘", 1000, 1000, 1000000},
		{"负数与正数相乘", -10, 2, -20},
		{"正数与负数相乘", 10, -2, -20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Multiply(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("Multiply(%d, %d) = %d; want %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}