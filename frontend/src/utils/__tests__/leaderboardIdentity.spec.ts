import { describe, expect, it } from 'vitest'
import { maskLeaderboardIdentity } from '../leaderboardIdentity'

describe('maskLeaderboardIdentity', () => {
  it.each([
    ['599155162@qq.com', 1, '59***62@qq.com'],
    ['ab@example.com', 2, 'a***b@example.com'],
    ['a@example.com', 3, 'a***@example.com'],
    ['用户名字@example.com', 4, '用***字@example.com'],
    ['59***62@qq.com', 5, '59***62@qq.com'],
    ['User #42', 6, 'User #6'],
    ['Anonymous #42', 7, 'Anonymous #7'],
  ])('masks %s', (value, rank, expected) => {
    expect(maskLeaderboardIdentity(value, rank)).toBe(expected)
  })
})
