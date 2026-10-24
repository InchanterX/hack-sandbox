import unittest

from powers import power


class PowerTest(unittest.TestCase):
    def test_raises_to_positive_power(self):
        self.assertEqual(power(2, 10), 1024)

    def test_zero_exponent_gives_one(self):
        self.assertEqual(power(3, 0), 1)

    def test_zero_exponent_gives_one_for_zero_base(self):
        self.assertEqual(power(0, 0), 1)

    def test_negative_exponent_raises(self):
        with self.assertRaises(ValueError):
            power(2, -1)


if __name__ == "__main__":
    unittest.main()
