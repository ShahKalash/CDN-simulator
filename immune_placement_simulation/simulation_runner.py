"""
Simulation Runner for Immune-Based Edge Placement
Runs multiple scenarios and collects comprehensive metrics
"""

import json
import time
import random
import numpy as np
from typing import Dict, List, Tuple
from immune_placement import (
    ImmunePlacementAlgorithm,
    generate_synthetic_network,
    Peer,
    RTTMatrix,
    Solution
)


class SimulationRunner:
    """Runs comprehensive simulations and collects metrics"""
    
    def __init__(self):
        self.scenarios = []
        self.results = []
    
    def run_baseline_comparison(self, num_peers: int = 160) -> Dict:
        """
        Run baseline (naive) vs optimized (immune) comparison
        
        Baseline strategies:
        1. Round-robin edge assignment
        2. Nearest edge assignment
        3. Random super peer selection
        """
        print("=" * 80)
        print("BASELINE COMPARISON SIMULATION")
        print("=" * 80)
        
        # Generate network
        peers, rtt_matrix = generate_synthetic_network(num_peers=num_peers, seed=42)
        
        results = {
            'scenario': 'baseline_comparison',
            'num_peers': num_peers,
            'baselines': {},
            'optimized': {}
        }
        
        # Baseline 1: Round-robin edge assignment
        print("\n[1/4] Baseline: Round-Robin Edge Assignment")
        round_robin_solution = self._round_robin_baseline(peers, rtt_matrix)
        results['baselines']['round_robin'] = self._evaluate_solution_metrics(
            round_robin_solution, peers, rtt_matrix
        )
        
        # Baseline 2: Nearest edge assignment
        print("\n[2/4] Baseline: Nearest Edge Assignment")
        nearest_solution = self._nearest_edge_baseline(peers, rtt_matrix)
        results['baselines']['nearest_edge'] = self._evaluate_solution_metrics(
            nearest_solution, peers, rtt_matrix
        )
        
        # Baseline 3: Random super peers
        print("\n[3/4] Baseline: Random Super Peers + Nearest Edge")
        random_super_solution = self._random_super_peers_baseline(peers, rtt_matrix, num_super_peers=15)
        results['baselines']['random_super_peers'] = self._evaluate_solution_metrics(
            random_super_solution, peers, rtt_matrix
        )
        
        # Optimized: Immune algorithm
        print("\n[4/4] Optimized: Immune-Based Algorithm")
        algorithm = ImmunePlacementAlgorithm(
            peers=peers,
            rtt_matrix=rtt_matrix,
            num_super_peers=15,
            alpha=0.7,
            beta=0.3,
            pop_size=30,
            max_generations=100
        )
        best_solution, history = algorithm.optimize()
        results['optimized'] = self._evaluate_solution_metrics(
            best_solution, peers, rtt_matrix
        )
        results['optimized']['convergence_history'] = history
        
        return results
    
    def _round_robin_baseline(self, peers: List[Peer], rtt_matrix: RTTMatrix) -> Solution:
        """Round-robin edge assignment baseline"""
        edge_assignments = {}
        for i, peer in enumerate(peers):
            edge_assignments[peer.id] = 'A' if i % 2 == 0 else 'B'
        
        return Solution(
            super_peers=[],
            edge_assignments=edge_assignments
        )
    
    def _nearest_edge_baseline(self, peers: List[Peer], rtt_matrix: RTTMatrix) -> Solution:
        """Assign each peer to nearest edge"""
        edge_assignments = {}
        for peer in peers:
            edge_assignments[peer.id] = 'A' if peer.rtt_to_edge_a < peer.rtt_to_edge_b else 'B'
        
        return Solution(
            super_peers=[],
            edge_assignments=edge_assignments
        )
    
    def _random_super_peers_baseline(self, peers: List[Peer], rtt_matrix: RTTMatrix, 
                                     num_super_peers: int) -> Solution:
        """Random super peer selection + nearest edge"""
        super_peers = random.sample([p.id for p in peers], num_super_peers)
        edge_assignments = {}
        for peer in peers:
            edge_assignments[peer.id] = 'A' if peer.rtt_to_edge_a < peer.rtt_to_edge_b else 'B'
        
        return Solution(
            super_peers=super_peers,
            edge_assignments=edge_assignments
        )
    
    def _evaluate_solution_metrics(self, solution: Solution, peers: List[Peer], 
                                   rtt_matrix: RTTMatrix) -> Dict:
        """Evaluate a solution and return comprehensive metrics"""
        peer_dict = {p.id: p for p in peers}
        
        effective_delays = []
        edge_loads = {'A': 0.0, 'B': 0.0}
        p2p_hits = 0
        edge_hits = {'A': 0, 'B': 0}
        total_demand = 0.0
        
        for peer_id, peer in peer_dict.items():
            total_demand += peer.demand
            
            # Check P2P option
            d_super = float('inf')
            if solution.super_peers:
                d_super = min(
                    rtt_matrix.peer_to_peer.get((peer_id, sp), 
                    rtt_matrix.peer_to_peer.get((sp, peer_id), 100.0))
                    for sp in solution.super_peers
                )
            
            # Edge option
            assigned_edge = solution.edge_assignments.get(peer_id, 'A')
            d_edge = peer.rtt_to_edge_a if assigned_edge == 'A' else peer.rtt_to_edge_b
            
            # Choose best
            if d_super + 5.0 < d_edge:  # 5ms P2P overhead
                d_effective = d_super + 5.0
                p2p_hits += 1
            else:
                d_effective = d_edge
                edge_hits[assigned_edge] += 1
            
            effective_delays.append((peer_id, d_effective, peer.demand))
            edge_loads[assigned_edge] += peer.demand
        
        # Compute metrics
        avg_delay = sum(d * r for _, d, r in effective_delays) / total_demand if total_demand > 0 else 0.0
        load_avg = (edge_loads['A'] + edge_loads['B']) / 2.0
        load_imbalance = 0.5 * ((edge_loads['A'] - load_avg)**2 + (edge_loads['B'] - load_avg)**2)
        
        # P2P hit rate
        p2p_hit_rate = p2p_hits / len(peers) if peers else 0.0
        
        return {
            'avg_delay_ms': avg_delay,
            'load_imbalance': load_imbalance,
            'edge_a_load_kbps': edge_loads['A'],
            'edge_b_load_kbps': edge_loads['B'],
            'edge_a_utilization': edge_loads['A'] / 10000.0,  # Assuming 10Gbps capacity
            'edge_b_utilization': edge_loads['B'] / 10000.0,
            'p2p_hit_rate': p2p_hit_rate,
            'edge_a_hits': edge_hits['A'],
            'edge_b_hits': edge_hits['B'],
            'p2p_hits': p2p_hits,
            'super_peers': solution.super_peers,
            'num_super_peers': len(solution.super_peers)
        }
    
    def run_parameter_sensitivity(self, num_peers: int = 160) -> Dict:
        """Test sensitivity to algorithm parameters"""
        print("=" * 80)
        print("PARAMETER SENSITIVITY ANALYSIS")
        print("=" * 80)
        
        peers, rtt_matrix = generate_synthetic_network(num_peers=num_peers, seed=42)
        
        results = {
            'scenario': 'parameter_sensitivity',
            'num_peers': num_peers,
            'alpha_beta_tests': [],
            'super_peer_tests': []
        }
        
        # Test different alpha/beta ratios
        print("\nTesting alpha/beta ratios...")
        for alpha in [0.5, 0.6, 0.7, 0.8, 0.9]:
            beta = 1.0 - alpha
            print(f"  Alpha={alpha:.1f}, Beta={beta:.1f}")
            
            algorithm = ImmunePlacementAlgorithm(
                peers=peers,
                rtt_matrix=rtt_matrix,
                num_super_peers=15,
                alpha=alpha,
                beta=beta,
                pop_size=20,
                max_generations=50  # Faster for sensitivity
            )
            best_solution, _ = algorithm.optimize()
            metrics = self._evaluate_solution_metrics(best_solution, peers, rtt_matrix)
            
            results['alpha_beta_tests'].append({
                'alpha': alpha,
                'beta': beta,
                'metrics': metrics
            })
        
        # Test different numbers of super peers
        print("\nTesting different numbers of super peers...")
        for num_sp in [5, 10, 15, 20, 25, 30]:
            print(f"  Super peers: {num_sp}")
            
            algorithm = ImmunePlacementAlgorithm(
                peers=peers,
                rtt_matrix=rtt_matrix,
                num_super_peers=num_sp,
                alpha=0.7,
                beta=0.3,
                pop_size=20,
                max_generations=50
            )
            best_solution, _ = algorithm.optimize()
            metrics = self._evaluate_solution_metrics(best_solution, peers, rtt_matrix)
            
            results['super_peer_tests'].append({
                'num_super_peers': num_sp,
                'metrics': metrics
            })
        
        return results
    
    def run_scalability_test(self) -> Dict:
        """Test scalability with different network sizes"""
        print("=" * 80)
        print("SCALABILITY ANALYSIS")
        print("=" * 80)
        
        results = {
            'scenario': 'scalability',
            'network_sizes': []
        }
        
        for num_peers in [50, 100, 160, 200, 300]:
            print(f"\nTesting with {num_peers} peers...")
            
            peers, rtt_matrix = generate_synthetic_network(num_peers=num_peers, seed=42)
            
            # Scale super peers proportionally
            num_super_peers = max(5, int(num_peers * 0.1))
            
            start_time = time.time()
            algorithm = ImmunePlacementAlgorithm(
                peers=peers,
                rtt_matrix=rtt_matrix,
                num_super_peers=num_super_peers,
                alpha=0.7,
                beta=0.3,
                pop_size=30,
                max_generations=50  # Reduced for scalability test
            )
            best_solution, _ = algorithm.optimize()
            elapsed_time = time.time() - start_time
            
            metrics = self._evaluate_solution_metrics(best_solution, peers, rtt_matrix)
            
            results['network_sizes'].append({
                'num_peers': num_peers,
                'num_super_peers': num_super_peers,
                'elapsed_time_seconds': elapsed_time,
                'metrics': metrics
            })
        
        return results
    
    def run_all_simulations(self) -> Dict:
        """Run all simulation scenarios"""
        print("\n" + "=" * 80)
        print("COMPREHENSIVE SIMULATION SUITE")
        print("=" * 80)
        
        all_results = {
            'timestamp': time.strftime('%Y-%m-%d %H:%M:%S'),
            'simulations': {}
        }
        
        # Run baseline comparison
        all_results['simulations']['baseline_comparison'] = self.run_baseline_comparison()
        
        # Run parameter sensitivity
        all_results['simulations']['parameter_sensitivity'] = self.run_parameter_sensitivity()
        
        # Run scalability test
        all_results['simulations']['scalability'] = self.run_scalability_test()
        
        # Save results
        with open('simulation_results.json', 'w') as f:
            json.dump(all_results, f, indent=2)
        
        print("\n" + "=" * 80)
        print("All simulations completed!")
        print("Results saved to simulation_results.json")
        print("=" * 80)
        
        return all_results


if __name__ == "__main__":
    runner = SimulationRunner()
    results = runner.run_all_simulations()

