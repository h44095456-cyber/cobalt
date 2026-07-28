package gui

import (
	"fmt"
	"math/rand"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
)

type Vector2D struct {
	X int32
	Y int32
}

func RunNativeRaylibSnakeGame() {
	const screenWidth int32 = 800
	const screenHeight int32 = 600
	const gridSize int32 = 20

	const tileCountX int32 = screenWidth / gridSize
	const tileCountY int32 = screenHeight / gridSize

	rl.InitWindow(screenWidth, screenHeight, "🐍 Cobalt Native GUI 2D Snake Game (Raylib Engine)")
	defer rl.CloseWindow()

	rl.SetExitKey(0) // Disable ESC key auto-close
	rl.SetTargetFPS(15)

	snake := []Vector2D{
		{X: 10, Y: 15},
		{X: 9, Y: 15},
		{X: 8, Y: 15},
	}

	dirX := int32(1)
	dirY := int32(0)

	rand.Seed(time.Now().UnixNano())
	food := Vector2D{
		X: int32(rand.Intn(int(tileCountX))),
		Y: int32(rand.Intn(int(tileCountY))),
	}

	score := 0
	gameOver := false

	for !rl.WindowShouldClose() {
		// Input Handling
		if (rl.IsKeyPressed(rl.KeyUp) || rl.IsKeyPressed(rl.KeyW)) && dirY != 1 {
			dirX = 0
			dirY = -1
		}
		if (rl.IsKeyPressed(rl.KeyDown) || rl.IsKeyPressed(rl.KeyS)) && dirY != -1 {
			dirX = 0
			dirY = 1
		}
		if (rl.IsKeyPressed(rl.KeyLeft) || rl.IsKeyPressed(rl.KeyA)) && dirX != 1 {
			dirX = -1
			dirY = 0
		}
		if (rl.IsKeyPressed(rl.KeyRight) || rl.IsKeyPressed(rl.KeyD)) && dirX != -1 {
			dirX = 1
			dirY = 0
		}

		if rl.IsKeyPressed(rl.KeyR) && gameOver {
			snake = []Vector2D{
				{X: 10, Y: 15},
				{X: 9, Y: 15},
				{X: 8, Y: 15},
			}
			dirX = 1
			dirY = 0
			score = 0
			gameOver = false
			food = Vector2D{X: int32(rand.Intn(int(tileCountX))), Y: int32(rand.Intn(int(tileCountY)))}
		}

		if !gameOver {
			// Update position
			head := Vector2D{
				X: snake[0].X + dirX,
				Y: snake[0].Y + dirY,
			}

			// Wall Collisions
			if head.X < 0 || head.X >= tileCountX || head.Y < 0 || head.Y >= tileCountY {
				gameOver = true
			}

			// Self Collisions
			for _, seg := range snake {
				if head.X == seg.X && head.Y == seg.Y {
					gameOver = true
					break
				}
			}

			if !gameOver {
				snake = append([]Vector2D{head}, snake...)

				// Check Food Collision
				if head.X == food.X && head.Y == food.Y {
					score += 10
					food = Vector2D{X: int32(rand.Intn(int(tileCountX))), Y: int32(rand.Intn(int(tileCountY)))}
				} else {
					snake = snake[:len(snake)-1]
				}
			}
		}

		// Rendering Frame
		rl.BeginDrawing()
		rl.ClearBackground(rl.NewColor(20, 24, 33, 255))

		if gameOver {
			rl.DrawText("GAME OVER", screenWidth/2-100, screenHeight/2-30, 36, rl.Red)
			rl.DrawText(fmt.Sprintf("Final Score: %d", score), screenWidth/2-75, screenHeight/2+20, 20, rl.White)
			rl.DrawText("Press [R] to Restart", screenWidth/2-95, screenHeight/2+50, 18, rl.Gray)
		} else {
			// Draw Food
			rl.DrawRectangle(food.X*gridSize, food.Y*gridSize, gridSize-2, gridSize-2, rl.Red)

			// Draw Snake Body
			for i, seg := range snake {
				color := rl.NewColor(46, 204, 113, 255)
				if i == 0 {
					color = rl.NewColor(39, 174, 96, 255)
				}
				rl.DrawRectangle(seg.X*gridSize, seg.Y*gridSize, gridSize-2, gridSize-2, color)
			}

			// Draw HUD Score
			rl.DrawText(fmt.Sprintf("Score: %d  |  Speed: 15 FPS  |  Controls: WASD / Arrows", score), 20, 20, 18, rl.RayWhite)
		}

		rl.EndDrawing()
	}
}

// Helper function for Int33 range
func (v Vector2D) Dummy() {}
