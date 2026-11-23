import unittest

from generalquarters import Battlefield, Unit, simulate_battle


class GameTests(unittest.TestCase):
    def test_units_take_damage_and_die(self):
        battlefield = Battlefield(8, 8)
        attacker = battlefield.add_unit(
            Unit("Rifle Squad", "Allies", "infantry", health=8, attack=5, armor=1),
            1,
            1,
        )
        defender = battlefield.add_unit(
            Unit("Axis Guard", "Axis", "infantry", health=6, attack=3, armor=1),
            1,
            1,
        )

        result = battlefield.resolve_attack(attacker.name, defender.name)

        self.assertEqual(result["damage"], 4)
        self.assertEqual(defender.health, 2)

    def test_battlefield_tracks_winner(self):
        battlefield = Battlefield(8, 8)
        allied = battlefield.add_unit(
            Unit("Sherman", "Allies", "tank", health=6, attack=5, armor=2),
            1,
            1,
        )
        axis = battlefield.add_unit(
            Unit("Tiger", "Axis", "tank", health=8, attack=7, armor=2),
            1,
            2,
        )

        battlefield.resolve_attack(allied.name, axis.name)
        battlefield.resolve_attack(axis.name, allied.name)

        self.assertIsNone(battlefield.winner())

        axis.take_damage(20)
        self.assertEqual(battlefield.winner(), "Allies")

    def test_simulate_battle_returns_result(self):
        result = simulate_battle()

        self.assertIn(result["winner"], {"Allies", "Axis", None})
        self.assertIsInstance(result["remaining_units"], list)


if __name__ == "__main__":
    unittest.main()
