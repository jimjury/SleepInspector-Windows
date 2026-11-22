package main

const (
	listViewItemChanged int32  = -101   // LVN_ITEMCHANGED
	listViewStateField  uint32 = 0x0008 // LVIF_STATE
	listViewSelectedBit uint32 = 0x0002 // LVIS_SELECTED
)

// Pure event filter, independently testable without a Windows desktop.
// Painting, scrolling, hover, and unchanged selection never request a UI refresh.
func shouldUpdateSelectedRow(code int32, changed, oldState, newState uint32) bool {
	return code == listViewItemChanged && changed&listViewStateField != 0 &&
		(oldState^newState)&listViewSelectedBit != 0
}
