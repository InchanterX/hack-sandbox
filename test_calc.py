import unittest

from calc import add, mul


class AddTest(unittest.TestCase):
    def test_adds_two_numbers(self):
        self.assertEqual(add(2, 3), 5)

    def test_adds_negative_numbers(self):
        self.assertEqual(add(-2, -3), -5)


class MulTest(unittest.TestCase):
    def test_multiplies_two_numbers(self):
        self.assertEqual(mul(2, 3), 6)

    def test_multiplies_by_zero(self):
        self.assertEqual(mul(7, 0), 0)


if __name__ == "__main__":
    unittest.main()
