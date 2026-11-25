// internal/spock/names_test.go
package spock

import "testing"

// Expected names come from spock.spock_gen_slot_name() on Spock 5.0.14 /
// PostgreSQL 18.7. The first cases cover the 16-byte limit, long node and
// database names, and characters Spock replaces; the rest cover every
// subscription name length from 17 to 48 bytes, so every hash tail length
// across one to four 12-byte blocks is exercised.
func TestGenSlotNameMatchesSpock(t *testing.T) {
	cases := []struct{ db, provider, sub, want string }{
		{"app", "n1", "sub_n1_n2", "spk_app_n1_sub_n1_n2"},
		{"my-db", "us-east-1", "sub_us_east_1_us_west_2", "spk_my_db_us_east_1_sub_us_ecd0c83f"},
		{"app", "node-alpha", "sub_node_alpha_node_beta", "spk_app_node_alpha_sub_node57e0ceb"},
		{"app", "node-a", "sub_node_a_node1", "spk_app_node_a_sub_node_a_node1"},
		{"app", "node-a", "sub_node_a_node_b", "spk_app_node_a_sub_node0fdac04"},
		{"app", "pgedge-us-east-1", "sub_pgedge_us_east_1_n2", "spk_app_pgedge_us_east_1_sub_pged47145da"},
		{"app", "pgedge-us-east-1a", "sub_pgedge_us_east_1a_n2", "spk_app_pgedge_u9015794_sub_pged90f7bed"},
		{"my_production_database", "n1", "sub_n1_n2", "spk_my_produ9f742b9_n1_sub_n1_n2"},
		{"my_production_db", "pgedge-region-west", "sub_pgedge_region_west_pgedge_region_east", "spk_my_production_db_pgedge_r368c19f_sub_pgeddbdfc9e"},
		{"app", "DC1-Primary", "sub_DC1_Primary_n2", "spk_app___1__rimary_sub___1_44ca302"},
		{"app", "n1", "sub_n1_node_alpha", "spk_app_n1_sub_n1_needc93c"},
		{"app", "n1", "sub_n1_us_central1", "spk_app_n1_sub_n1_u98f4229"},
		{"app", "n1", "sub_n1_europe_west4", "spk_app_n1_sub_n1_e5124183"},
		{"app", "n1", "sub_n1_asia_south1_a", "spk_app_n1_sub_n1_ab4f93b0"},
		{"app", "n1", "sub_n1_pgedge_node_01", "spk_app_n1_sub_n1_pbd103cb"},
		{"app", "n1", "sub_n1_ap_southeast_2b", "spk_app_n1_sub_n1_a6fd727d"},
		{"app", "n1", "sub_n1_pgedge_us_east_1", "spk_app_n1_sub_n1_p22b6039"},
		{"app", "n1", "sub_n1_pgedge_us_east_1a", "spk_app_n1_sub_n1_pa7654cc"},
		{"app", "n1", "sub_n1_pgedge_eu_west_2_b", "spk_app_n1_sub_n1_p4b5d350"},
		{"app", "n1", "sub_n1_pgedge_eu_central_1", "spk_app_n1_sub_n1_pb2be6ac"},
		{"app", "n10", "sub_n10_pgedge_eu_central_1", "spk_app_n10_sub_n10_ea4a2cb"},
		{"app", "n1", "sub_n1_pgedge_ap_southeast_1", "spk_app_n1_sub_n1_p7da18fe"},
		{"app", "n10", "sub_n10_pgedge_ap_southeast_1", "spk_app_n10_sub_n10_8983818"},
		{"app", "n1", "sub_n1_pgedge_prod_us_central1", "spk_app_n1_sub_n1_p45bb559"},
		{"app", "n1", "sub_n1_pgedge_prod_europe_west4", "spk_app_n1_sub_n1_pcada3ff"},
		{"app", "n10", "sub_n10_pgedge_prod_europe_west4", "spk_app_n10_sub_n10_9373bc9"},
		{"app", "east", "sub_east_pgedge_prod_europe_west4", "spk_app_east_sub_east20c4816"},
		{"app", "node1", "sub_node1_pgedge_prod_europe_west4", "spk_app_node1_sub_node8d3e1b0"},
		{"app", "node-a", "sub_node_a_pgedge_prod_europe_west4", "spk_app_node_a_sub_node80583bb"},
		{"app", "us-east", "sub_us_east_pgedge_prod_europe_west4", "spk_app_us_east_sub_us_e9a72cdf"},
		{"app", "eu-west1", "sub_eu_west1_pgedge_prod_europe_west4", "spk_app_eu_west1_sub_eu_w3bb4b1c"},
		{"app", "us-east-1", "sub_us_east_1_pgedge_prod_europe_west4", "spk_app_us_east_1_sub_us_eb6a5819"},
		{"app", "node-alpha", "sub_node_alpha_pgedge_prod_europe_west4", "spk_app_node_alpha_sub_node457efc4"},
		{"app", "us-central1", "sub_us_central1_pgedge_prod_europe_west4", "spk_app_us_central1_sub_us_c2f7b442"},
		{"app", "europe-west4", "sub_europe_west4_pgedge_prod_europe_west4", "spk_app_europe_west4_sub_euro0141a67"},
		{"app", "asia-south1-a", "sub_asia_south1_a_pgedge_prod_europe_west4", "spk_app_asia_south1_a_sub_asia5f77aba"},
		{"app", "pgedge-node-01", "sub_pgedge_node_01_pgedge_prod_europe_west4", "spk_app_pgedge_node_01_sub_pged173e669"},
		{"app", "ap-southeast-2b", "sub_ap_southeast_2b_pgedge_prod_europe_west4", "spk_app_ap_southeast_2b_sub_ap_s4de75e9"},
		{"app", "pgedge-us-east-1", "sub_pgedge_us_east_1_pgedge_prod_europe_west4", "spk_app_pgedge_us_east_1_sub_pgedd927e34"},
		{"app", "pgedge-us-east-1a", "sub_pgedge_us_east_1a_pgedge_prod_europe_west4", "spk_app_pgedge_u9015794_sub_pgedb914757"},
		{"app", "pgedge-eu-west-2-b", "sub_pgedge_eu_west_2_b_pgedge_prod_europe_west4", "spk_app_pgedge_e0b2b3f9_sub_pged146372a"},
		{"app", "pgedge-eu-central-1", "sub_pgedge_eu_central_1_pgedge_prod_europe_west4", "spk_app_pgedge_ee981be6_sub_pged456ceb6"},
	}
	for _, c := range cases {
		if got := genSlotName(c.db, c.provider, c.sub); got != c.want {
			t.Errorf("genSlotName(%q, %q, %q) = %q, want %q", c.db, c.provider, c.sub, got, c.want)
		}
	}
}

// spockSlotName must agree with Spock for the subscription names we create.
func TestSpockSlotNameLongNodeNames(t *testing.T) {
	if got, want := spockSlotName("app", "node-alpha", "node-beta"), "spk_app_node_alpha_sub_node57e0ceb"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
